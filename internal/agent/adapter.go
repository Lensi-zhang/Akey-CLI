package agent

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/mcp"
)

var (
	ErrConflict         = errors.New("agent configuration conflict")
	ErrNotFound         = errors.New("agent target not found")
	ErrInvalidConfig    = errors.New("invalid agent configuration")
	ErrCodexUnavailable = errors.New("codex CLI is unavailable")
	ErrCodexCommand     = errors.New("codex MCP command failed")
	ErrCodexOutput      = errors.New("codex returned incompatible MCP output")
	ErrAdapterExists    = errors.New("agent adapter already registered")
	ErrAdapterNotFound  = errors.New("agent adapter not found")
)

const (
	ActionConfigure     = "configure"
	ActionAdd           = "add"
	ActionRemove        = "remove"
	ActionRenameProfile = "rename-profile"
)

// Adapter 仅配置启动元数据。实现不得接收或读取 API Key；
// 凭证由 akey run 在后续启动时注入。
type Adapter interface {
	Name() string
	Detect() (InstallInfo, error)
	Status() (Status, error)
	Configure(profile config.Profile) (ChangeSet, error)
	AddMCP(profile config.Profile, server mcp.Server) (ChangeSet, error)
	RemoveMCP(serverName string) (ChangeSet, error)
	ListMCP() (mcp.Document, error)
	RenameProfile(oldName string, profile config.Profile) ([]ChangeSet, error)
}

// InstallInfo 仅包含非敏感的安装与路径元数据。
type InstallInfo struct {
	Name           string `json:"name"`
	Installed      bool   `json:"installed"`
	ExecutablePath string `json:"executable_path,omitempty"`
	ConfigPath     string `json:"config_path,omitempty"`
	ConfigExists   bool   `json:"config_exists"`
}

// Status 是 Agent MCP 配置的非敏感状态快照。
type Status struct {
	Name           string   `json:"name"`
	Installed      bool     `json:"installed"`
	Configured     bool     `json:"configured"`
	ConfigPath     string   `json:"config_path,omitempty"`
	Profiles       []string `json:"profiles"`
	MCPCount       int      `json:"mcp_count"`
	ExecutablePath string   `json:"executable_path,omitempty"`
}

// ChangeSet 描述已完成的元数据变更，不包含凭证或 Provider 连接配置。
type ChangeSet struct {
	Agent      string `json:"agent"`
	Action     string `json:"action"`
	Changed    bool   `json:"changed"`
	ConfigPath string `json:"config_path,omitempty"`
	BackupPath string `json:"backup_path,omitempty"`
	ServerName string `json:"server_name,omitempty"`
}

// Registry 按稳定命令名称保存适配器。
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

// NewDefaultRegistry 注册 Trae、Claude Code 与 Codex。
// 构造注册表时不访问文件系统、凭证或子进程。
func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	_ = registry.Register(NewTrae(""))
	_ = registry.Register(NewClaudeCode(""))
	_ = registry.Register(NewCodex(nil))
	return registry
}

func (r *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return errors.New("cannot register a nil agent adapter")
	}
	name := adapter.Name()
	if name == "" {
		return errors.New("cannot register an agent adapter with an empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapters == nil {
		r.adapters = make(map[string]Adapter)
	}
	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("%w: %s", ErrAdapterExists, name)
	}
	r.adapters[name] = adapter
	return nil
}

func (r *Registry) Get(name string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAdapterNotFound, name)
	}
	return adapter, nil
}

func (r *Registry) List() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Adapter, 0, len(names))
	for _, name := range names {
		result = append(result, r.adapters[name])
	}
	return result
}

// RenameProfile 按适配器名称顺序同步 akey 管理的 MCP Profile 引用。
// 如果某个适配器失败，返回值包含失败前已经完成的变更，不进行跨 Agent 回滚。
func (r *Registry) RenameProfile(oldName string, profile config.Profile) ([]ChangeSet, error) {
	if err := mcp.ValidateName(oldName); err != nil {
		return nil, fmt.Errorf("invalid old profile: %w", err)
	}
	if err := mcp.ValidateName(profile.Name); err != nil {
		return nil, fmt.Errorf("invalid new profile: %w", err)
	}
	changes := []ChangeSet{}
	if oldName == profile.Name {
		return changes, nil
	}
	for _, adapter := range r.List() {
		adapterChanges, err := adapter.RenameProfile(oldName, profile)
		changes = append(changes, adapterChanges...)
		if err != nil {
			return changes, fmt.Errorf("rename profile in %s: %w", adapter.Name(), err)
		}
	}
	return changes, nil
}
