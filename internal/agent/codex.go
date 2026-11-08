package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/mcp"
)

// CommandRunner 抽象可执行文件查找与输出捕获，便于隔离测试 Codex CLI。
type CommandRunner interface {
	LookPath(file string) (string, error)
	Output(path string, args ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (execCommandRunner) Output(path string, args ...string) ([]byte, error) {
	return exec.Command(path, args...).Output()
}

// Codex 仅委托官方 codex mcp 命令，不读取或修改 TOML。
type Codex struct {
	runner CommandRunner
}

// NewCodex 创建 Codex 适配器；runner 为空时使用操作系统进程实现。
func NewCodex(runner CommandRunner) *Codex {
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &Codex{runner: runner}
}

func (c *Codex) Name() string {
	return "codex"
}

func (c *Codex) Detect() (InstallInfo, error) {
	path, err := c.runner.LookPath("codex")
	if err != nil || path == "" {
		return InstallInfo{Name: "codex"}, nil
	}
	return InstallInfo{Name: "codex", Installed: true, ExecutablePath: path}, nil
}

func (c *Codex) Status() (Status, error) {
	detected, err := c.Detect()
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Name:           "codex",
		Installed:      detected.Installed,
		Profiles:       []string{},
		ExecutablePath: detected.ExecutablePath,
	}
	if !detected.Installed {
		return status, nil
	}
	document, err := c.ListMCP()
	if err != nil {
		return Status{}, err
	}
	status.Configured = len(document.MCPServers) > 0
	status.MCPCount = len(document.MCPServers)
	status.Profiles = profilesFromDocument(document)
	return status, nil
}

// Configure 不写 Codex 全局配置；Codex 的 MCP 启动元数据由 AddMCP 设置。
func (c *Codex) Configure(profile config.Profile) (ChangeSet, error) {
	if err := mcp.ValidateName(profile.Name); err != nil {
		return ChangeSet{}, fmt.Errorf("invalid profile: %w", err)
	}
	return ChangeSet{Agent: "codex", Action: ActionConfigure}, nil
}

func (c *Codex) AddMCP(profile config.Profile, server mcp.Server) (ChangeSet, error) {
	if err := mcp.ValidateName(profile.Name); err != nil {
		return ChangeSet{}, fmt.Errorf("invalid profile: %w", err)
	}
	if err := server.Validate(); err != nil {
		return ChangeSet{}, err
	}
	if server.Profile != profile.Name {
		return ChangeSet{}, fmt.Errorf("%w: server profile does not match selected profile", ErrConflict)
	}
	entry, err := server.Entry()
	if err != nil {
		return ChangeSet{}, err
	}
	args := []string{"mcp", "add", server.Name, "--", entry.Command}
	args = append(args, entry.Args...)
	change := ChangeSet{Agent: "codex", Action: ActionAdd}
	if _, err := c.run("add", args...); err != nil {
		return change, err
	}
	change.Changed = true
	return change, nil
}

// RemoveMCP 先通过 get 校验所有权；交互确认由上层命令负责。
func (c *Codex) RemoveMCP(serverName string) (ChangeSet, error) {
	if err := mcp.ValidateName(serverName); err != nil {
		return ChangeSet{}, err
	}
	change := ChangeSet{Agent: "codex", Action: ActionRemove}
	output, err := c.run("get", "mcp", "get", serverName, "--json")
	if err != nil {
		return change, err
	}
	entry, err := parseCodexGet(output, serverName)
	if err != nil {
		return change, err
	}
	if !mcp.IsManaged(entry) {
		return change, fmt.Errorf("%w: refusing to remove external MCP server %q", ErrConflict, serverName)
	}
	if _, err := c.run("remove", "mcp", "remove", serverName); err != nil {
		return change, err
	}
	change.Changed = true
	return change, nil
}

func (c *Codex) ListMCP() (mcp.Document, error) {
	output, err := c.run("list", "mcp", "list", "--json")
	if err != nil {
		return mcp.Document{}, err
	}
	return parseCodexList(output)
}

// RenameProfile 通过官方 remove/add 命令逐项更新严格匹配的 stdio 条目。
// 失败时返回已完成的操作；上层可据此提示 Codex 可能存在部分更新。
func (c *Codex) RenameProfile(oldName string, profile config.Profile) ([]ChangeSet, error) {
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
	document, err := c.ListMCP()
	if err != nil {
		if errors.Is(err, ErrCodexUnavailable) {
			exists, configErr := codexConfigExists()
			if configErr != nil {
				return nil, configErr
			}
			if !exists {
				return changes, nil
			}
		}
		return nil, err
	}
	names := make([]string, 0, len(document.MCPServers))
	for name := range document.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := document.MCPServers[name]
		profileName, managed := mcp.ManagedProfile(entry)
		if !managed || profileName != oldName {
			continue
		}
		output, err := c.run("get", "mcp", "get", name, "--json")
		if err != nil {
			return changes, err
		}
		currentEntry, err := parseCodexGet(output, name)
		if err != nil {
			return changes, err
		}
		currentProfile, managed := mcp.ManagedProfile(currentEntry)
		if !managed || currentProfile != oldName {
			return changes, fmt.Errorf("%w: MCP server %q changed before profile rename", ErrConflict, name)
		}
		rawEntry, err := codexGetRawEntry(output, name)
		if err != nil {
			return changes, err
		}
		if !codexEntryHasOnlyStdioFields(rawEntry, 0) {
			return changes, fmt.Errorf("%w: refusing to rename MCP server %q with extended fields", ErrConflict, name)
		}
		server, err := mcp.ServerFromEntry(name, currentEntry)
		if err != nil {
			return changes, ErrCodexOutput
		}
		server.Profile = profile.Name
		if _, err := c.run("remove", "mcp", "remove", name); err != nil {
			return changes, err
		}
		changes = append(changes, ChangeSet{
			Agent:      "codex",
			Action:     ActionRemove,
			Changed:    true,
			ServerName: name,
		})
		updatedEntry, err := server.Entry()
		if err != nil {
			return changes, ErrCodexOutput
		}
		args := []string{"mcp", "add", name, "--", updatedEntry.Command}
		args = append(args, updatedEntry.Args...)
		if _, err := c.run("add", args...); err != nil {
			return changes, err
		}
		changes = append(changes, ChangeSet{
			Agent:      "codex",
			Action:     ActionAdd,
			Changed:    true,
			ServerName: name,
		})
	}
	return changes, nil
}

// codexConfigExists 只检查默认配置是否存在，不读取或解析 TOML。
func codexConfigExists() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("resolve Codex configuration path: %w", err)
	}
	_, err = os.Stat(filepath.Join(home, ".codex", "config.toml"))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("inspect Codex configuration: %w", err)
}

func (c *Codex) run(operation string, args ...string) ([]byte, error) {
	path, err := c.runner.LookPath("codex")
	if err != nil || path == "" {
		return nil, fmt.Errorf("%w: install Codex and ensure codex is on PATH", ErrCodexUnavailable)
	}
	output, err := c.runner.Output(path, args...)
	if err != nil {
		// 第三方 stdout、stderr 和错误文本可能含敏感值，不能拼入错误。
		return nil, fmt.Errorf("%w: %s", ErrCodexCommand, operation)
	}
	return output, nil
}

func parseCodexList(output []byte) (mcp.Document, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return mcp.Document{}, ErrCodexOutput
	}
	if trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return mcp.Document{}, ErrCodexOutput
		}
		document := mcp.EmptyDocument()
		for _, item := range items {
			name, entry, err := parseCodexEntry(item, "", 0)
			if err != nil {
				return mcp.Document{}, ErrCodexOutput
			}
			if _, exists := document.MCPServers[name]; exists {
				return mcp.Document{}, ErrCodexOutput
			}
			document.MCPServers[name] = entry
		}
		return document, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil || object == nil {
		return mcp.Document{}, ErrCodexOutput
	}
	if rawServers, exists := object["mcpServers"]; exists {
		return parseCodexServerMap(rawServers)
	}
	if hasCodexEntryShape(object) {
		name, entry, err := parseCodexEntry(trimmed, "", 0)
		if err != nil {
			return mcp.Document{}, ErrCodexOutput
		}
		document := mcp.EmptyDocument()
		document.MCPServers[name] = entry
		return document, nil
	}
	rawServers, err := json.Marshal(object)
	if err != nil {
		return mcp.Document{}, ErrCodexOutput
	}
	return parseCodexServerMap(rawServers)
}

func parseCodexGet(output []byte, expectedName string) (mcp.Entry, error) {
	rawEntry, err := codexGetRawEntry(output, expectedName)
	if err != nil {
		return mcp.Entry{}, err
	}
	name, entry, err := parseCodexEntry(rawEntry, expectedName, 0)
	if err != nil || name != expectedName {
		return mcp.Entry{}, ErrCodexOutput
	}
	return entry, nil
}

func codexGetRawEntry(output []byte, expectedName string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return nil, ErrCodexOutput
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil || object == nil {
		return nil, ErrCodexOutput
	}
	if rawServers, exists := object["mcpServers"]; exists {
		var servers map[string]json.RawMessage
		if json.Unmarshal(rawServers, &servers) != nil || servers == nil {
			return nil, ErrCodexOutput
		}
		rawEntry, exists := servers[expectedName]
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, expectedName)
		}
		return rawEntry, nil
	}
	if rawEntry, exists := object[expectedName]; exists && !hasCodexEntryShape(object) {
		return rawEntry, nil
	}
	return trimmed, nil
}

// codexEntryHasOnlyStdioFields 判断官方 remove/add 是否能无丢失地重建条目。
func codexEntryHasOnlyStdioFields(raw json.RawMessage, depth int) bool {
	if depth > 2 {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return false
	}
	if _, hasCommand := fields["command"]; !hasCommand {
		for key := range fields {
			if key != "name" && key != "config" && key != "server" {
				return false
			}
		}
		for _, key := range []string{"config", "server"} {
			if nested, exists := fields[key]; exists {
				return codexEntryHasOnlyStdioFields(nested, depth+1)
			}
		}
		return false
	}
	for key := range fields {
		switch key {
		case "name", "command", "args", "type":
		default:
			return false
		}
	}
	return true
}

func parseCodexServerMap(raw json.RawMessage) (mcp.Document, error) {
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(raw, &servers); err != nil || servers == nil {
		return mcp.Document{}, ErrCodexOutput
	}
	document := mcp.EmptyDocument()
	for name, rawEntry := range servers {
		actualName, entry, err := parseCodexEntry(rawEntry, name, 0)
		if err != nil || actualName != name {
			return mcp.Document{}, ErrCodexOutput
		}
		document.MCPServers[name] = entry
	}
	return document, nil
}

func hasCodexEntryShape(object map[string]json.RawMessage) bool {
	for _, key := range []string{"command", "config", "server", "name"} {
		if _, exists := object[key]; exists {
			return true
		}
	}
	return false
}

func parseCodexEntry(raw json.RawMessage, nameHint string, depth int) (string, mcp.Entry, error) {
	if depth > 2 {
		return "", mcp.Entry{}, ErrCodexOutput
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return "", mcp.Entry{}, ErrCodexOutput
	}
	name := nameHint
	if rawName, exists := fields["name"]; exists {
		if err := json.Unmarshal(rawName, &name); err != nil {
			return "", mcp.Entry{}, ErrCodexOutput
		}
		if nameHint != "" && name != nameHint {
			return "", mcp.Entry{}, ErrCodexOutput
		}
	}
	if _, hasCommand := fields["command"]; !hasCommand {
		for _, key := range []string{"config", "server"} {
			if nested, exists := fields[key]; exists {
				return parseCodexEntry(nested, name, depth+1)
			}
		}
	}
	if err := mcp.ValidateName(name); err != nil {
		return "", mcp.Entry{}, ErrCodexOutput
	}
	var entry mcp.Entry
	rawCommand, exists := fields["command"]
	if !exists || json.Unmarshal(rawCommand, &entry.Command) != nil || strings.TrimSpace(entry.Command) == "" {
		return "", mcp.Entry{}, ErrCodexOutput
	}
	entry.Args = []string{}
	if rawArgs, exists := fields["args"]; exists {
		if err := json.Unmarshal(rawArgs, &entry.Args); err != nil || entry.Args == nil {
			return "", mcp.Entry{}, ErrCodexOutput
		}
	}
	if rawType, exists := fields["type"]; exists {
		if err := json.Unmarshal(rawType, &entry.Type); err != nil {
			return "", mcp.Entry{}, ErrCodexOutput
		}
	}
	entry.Args = append([]string(nil), entry.Args...)
	return name, entry, nil
}

// 确保默认运行器持续满足可注入接口。
var _ CommandRunner = execCommandRunner{}
