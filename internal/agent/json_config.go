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

type jsonAdapter struct {
	agentName  string
	executable string
	stdio      bool
	path       string
}

type jsonConfig struct {
	root    map[string]json.RawMessage
	servers map[string]json.RawMessage
	data    []byte
	exists  bool
}

func DefaultTraePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for Trae: %w", err)
	}
	return filepath.Join(home, ".trae", "mcp.json"), nil
}

func DefaultClaudeCodePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for Claude Code: %w", err)
	}
	return filepath.Join(home, ".claude.json"), nil
}

func (a *jsonAdapter) Name() string {
	return a.agentName
}

func (a *jsonAdapter) configPath() (string, error) {
	if a.path != "" {
		return a.path, nil
	}
	if a.agentName == "trae" {
		return DefaultTraePath()
	}
	return DefaultClaudeCodePath()
}

func (a *jsonAdapter) Detect() (InstallInfo, error) {
	path, err := a.configPath()
	if err != nil {
		return InstallInfo{}, err
	}
	info := InstallInfo{Name: a.agentName, ConfigPath: path}
	executable, lookErr := exec.LookPath(a.executable)
	if lookErr == nil {
		info.Installed = true
		info.ExecutablePath = executable
	} else if !errors.Is(lookErr, exec.ErrNotFound) {
		return info, fmt.Errorf("detect %s executable: %w", a.agentName, lookErr)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		info.ConfigExists = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return info, fmt.Errorf("inspect %s configuration: %w", a.agentName, statErr)
	}
	return info, nil
}

func (a *jsonAdapter) Status() (Status, error) {
	detected, err := a.Detect()
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Name:           a.agentName,
		Installed:      detected.Installed,
		ConfigPath:     detected.ConfigPath,
		Profiles:       []string{},
		ExecutablePath: detected.ExecutablePath,
	}
	if !detected.ConfigExists {
		return status, nil
	}
	loaded, err := loadJSONConfig(detected.ConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return status, nil
		}
		return Status{}, err
	}
	status.Configured = loaded.exists
	document, err := documentFromRaw(loaded.servers)
	if err != nil {
		return Status{}, err
	}
	status.MCPCount = len(document.MCPServers)
	status.Profiles = profilesFromDocument(document)
	return status, nil
}

func (a *jsonAdapter) Configure(profile config.Profile) (ChangeSet, error) {
	if err := mcp.ValidateName(profile.Name); err != nil {
		return ChangeSet{}, fmt.Errorf("invalid profile: %w", err)
	}
	return a.update(ActionConfigure, func(loaded *jsonConfig) (bool, error) {
		_, exists := loaded.root["mcpServers"]
		return !loaded.exists || !exists, nil
	})
}

func (a *jsonAdapter) AddMCP(profile config.Profile, server mcp.Server) (ChangeSet, error) {
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
	if a.stdio {
		entry.Type = "stdio"
	}
	rawEntry, err := json.Marshal(entry)
	if err != nil {
		return ChangeSet{}, fmt.Errorf("encode MCP entry: %w", err)
	}
	return a.update(ActionAdd, func(loaded *jsonConfig) (bool, error) {
		if _, exists := loaded.servers[server.Name]; exists {
			return false, fmt.Errorf("%w: MCP server %q already exists", ErrConflict, server.Name)
		}
		loaded.servers[server.Name] = rawEntry
		return true, nil
	})
}

func (a *jsonAdapter) RemoveMCP(serverName string) (ChangeSet, error) {
	if err := mcp.ValidateName(serverName); err != nil {
		return ChangeSet{}, err
	}
	return a.update(ActionRemove, func(loaded *jsonConfig) (bool, error) {
		rawEntry, exists := loaded.servers[serverName]
		if !exists {
			return false, nil
		}
		entry, err := decodeEntry(rawEntry)
		if err != nil {
			return false, invalidConfigError(serverName, err)
		}
		if !mcp.IsManaged(entry) {
			return false, fmt.Errorf("%w: refusing to remove external MCP server %q", ErrConflict, serverName)
		}
		delete(loaded.servers, serverName)
		return true, nil
	})
}

func (a *jsonAdapter) ListMCP() (mcp.Document, error) {
	path, err := a.configPath()
	if err != nil {
		return mcp.Document{}, err
	}
	loaded, err := loadJSONConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return mcp.EmptyDocument(), nil
		}
		return mcp.Document{}, err
	}
	return documentFromRaw(loaded.servers)
}

// RenameProfile 仅改写严格匹配 oldName 的 akey 参数，其他字段保持原值。
func (a *jsonAdapter) RenameProfile(oldName string, profile config.Profile) ([]ChangeSet, error) {
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
	change, err := a.update(ActionRenameProfile, func(loaded *jsonConfig) (bool, error) {
		changed := false
		for name, rawEntry := range loaded.servers {
			entry, err := decodeEntry(rawEntry)
			if err != nil {
				return false, invalidConfigError(name, err)
			}
			profileName, managed := mcp.ManagedProfile(entry)
			if !managed || profileName != oldName {
				continue
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(rawEntry, &fields); err != nil || fields == nil {
				return false, invalidConfigError(name, errors.New("entry must be an object"))
			}
			entry.Args = append([]string(nil), entry.Args...)
			entry.Args[2] = profile.Name
			rawArgs, err := json.Marshal(entry.Args)
			if err != nil {
				return false, fmt.Errorf("%w: encode renamed args: %v", ErrInvalidConfig, err)
			}
			fields["args"] = rawArgs
			updatedEntry, err := json.Marshal(fields)
			if err != nil {
				return false, fmt.Errorf("%w: encode renamed entry: %v", ErrInvalidConfig, err)
			}
			loaded.servers[name] = updatedEntry
			changed = true
		}
		return changed, nil
	})
	if err != nil {
		return nil, err
	}
	if change.Changed {
		changes = append(changes, change)
	}
	return changes, nil
}

func (a *jsonAdapter) update(action string, mutate func(*jsonConfig) (bool, error)) (ChangeSet, error) {
	path, err := a.configPath()
	if err != nil {
		return ChangeSet{}, err
	}
	change := ChangeSet{Agent: a.agentName, Action: action, ConfigPath: path}
	loaded, err := loadJSONConfig(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return change, err
	}
	if errors.Is(err, os.ErrNotExist) {
		loaded = emptyJSONConfig()
	}
	changed, err := mutate(loaded)
	if err != nil || !changed {
		return change, err
	}
	rawServers, err := json.Marshal(loaded.servers)
	if err != nil {
		return change, fmt.Errorf("%w: encode mcpServers: %v", ErrInvalidConfig, err)
	}
	loaded.root["mcpServers"] = rawServers
	output, err := json.MarshalIndent(loaded.root, "", "  ")
	if err != nil {
		return change, fmt.Errorf("%w: encode configuration: %v", ErrInvalidConfig, err)
	}
	output = append(output, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return change, fmt.Errorf("create agent configuration directory: %w", err)
	}
	if loaded.exists {
		backupPath, err := writeBackup(path, loaded.data)
		if err != nil {
			return change, err
		}
		change.BackupPath = backupPath
	}
	if err := atomicWrite(path, output); err != nil {
		return change, err
	}
	change.Changed = true
	return change, nil
}

func emptyJSONConfig() *jsonConfig {
	return &jsonConfig{
		root:    make(map[string]json.RawMessage),
		servers: make(map[string]json.RawMessage),
	}
}

func loadJSONConfig(path string) (*jsonConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: %s is empty", ErrInvalidConfig, path)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%w: parse %s: %v", ErrInvalidConfig, path, err)
	}
	if root == nil {
		return nil, fmt.Errorf("%w: %s must contain a JSON object", ErrInvalidConfig, path)
	}
	servers := make(map[string]json.RawMessage)
	if rawServers, exists := root["mcpServers"]; exists {
		if bytes.Equal(bytes.TrimSpace(rawServers), []byte("null")) {
			return nil, fmt.Errorf("%w: mcpServers must be an object", ErrInvalidConfig)
		}
		if err := json.Unmarshal(rawServers, &servers); err != nil {
			return nil, fmt.Errorf("%w: mcpServers must be an object: %v", ErrInvalidConfig, err)
		}
		if servers == nil {
			return nil, fmt.Errorf("%w: mcpServers must be an object", ErrInvalidConfig)
		}
	}
	for name, rawEntry := range servers {
		if err := mcp.ValidateName(name); err != nil {
			return nil, fmt.Errorf("%w: invalid server name %q", ErrInvalidConfig, name)
		}
		if _, err := decodeEntry(rawEntry); err != nil {
			return nil, invalidConfigError(name, err)
		}
	}
	return &jsonConfig{root: root, servers: servers, data: data, exists: true}, nil
}

func invalidConfigError(name string, err error) error {
	return fmt.Errorf("%w: invalid MCP server %q: %v", ErrInvalidConfig, name, err)
}

func decodeEntry(raw json.RawMessage) (mcp.Entry, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return mcp.Entry{}, errors.New("entry must be an object")
	}
	if fields == nil {
		return mcp.Entry{}, errors.New("entry must be an object")
	}
	var entry mcp.Entry
	rawCommand, exists := fields["command"]
	if !exists || json.Unmarshal(rawCommand, &entry.Command) != nil || strings.TrimSpace(entry.Command) == "" {
		return mcp.Entry{}, errors.New("command must be a non-empty string")
	}
	entry.Args = []string{}
	if rawArgs, exists := fields["args"]; exists {
		if err := json.Unmarshal(rawArgs, &entry.Args); err != nil || entry.Args == nil {
			return mcp.Entry{}, errors.New("args must be an array of strings")
		}
	}
	if rawType, exists := fields["type"]; exists {
		if err := json.Unmarshal(rawType, &entry.Type); err != nil {
			return mcp.Entry{}, errors.New("type must be a string")
		}
	}
	return entry, nil
}

func documentFromRaw(servers map[string]json.RawMessage) (mcp.Document, error) {
	document := mcp.EmptyDocument()
	for name, rawEntry := range servers {
		entry, err := decodeEntry(rawEntry)
		if err != nil {
			return mcp.Document{}, invalidConfigError(name, err)
		}
		entry.Args = append([]string(nil), entry.Args...)
		if entry.Args == nil {
			entry.Args = []string{}
		}
		document.MCPServers[name] = entry
	}
	return document, nil
}

func profilesFromDocument(document mcp.Document) []string {
	unique := make(map[string]struct{})
	for _, entry := range document.MCPServers {
		if profile, managed := mcp.ManagedProfile(entry); managed {
			unique[profile] = struct{}{}
		}
	}
	profiles := make([]string, 0, len(unique))
	for profile := range unique {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)
	return profiles
}

func writeBackup(path string, data []byte) (string, error) {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+".backup-*")
	if err != nil {
		return "", fmt.Errorf("create configuration backup: %w", err)
	}
	backupPath := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(backupPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", fmt.Errorf("protect configuration backup: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("write configuration backup: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("flush configuration backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close configuration backup: %w", err)
	}
	ok = true
	return backupPath, nil
}

func atomicWrite(path string, data []byte) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary agent configuration: %w", err)
	}
	temporaryPath := file.Name()
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect temporary agent configuration: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write temporary agent configuration: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("flush temporary agent configuration: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary agent configuration: %w", err)
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("atomically replace agent configuration: %w", err)
	}
	committed = true
	return nil
}
