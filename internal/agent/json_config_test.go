package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/mcp"
)

func TestTraeConfigureCreatesEmptyConfigWithoutProfileMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "mcp.json")
	adapter := NewTrae(path)
	profile := config.Profile{
		Name:     "volc-prod",
		Provider: "provider-must-not-be-written",
		BaseURL:  "https://example.invalid",
		Model:    "model-must-not-be-written",
	}
	change, err := adapter.Configure(profile)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	if !change.Changed || change.ConfigPath != path || change.BackupPath != "" {
		t.Fatalf("Configure() change = %#v", change)
	}
	data := mustReadFile(t, path)
	var document mcp.Document
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("created config is invalid: %v", err)
	}
	if document.MCPServers == nil || len(document.MCPServers) != 0 {
		t.Fatalf("mcpServers = %#v, want empty object", document.MCPServers)
	}
	for _, forbidden := range []string{profile.Provider, profile.BaseURL, profile.Model, "api_key", "apiKey", "password"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("configuration contains forbidden value %q: %s", forbidden, data)
		}
	}
	second, err := adapter.Configure(profile)
	if err != nil {
		t.Fatalf("second Configure() error = %v", err)
	}
	if second.Changed || second.BackupPath != "" {
		t.Fatalf("second Configure() = %#v, want no change", second)
	}
	status, err := adapter.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Configured || status.MCPCount != 0 || len(status.Profiles) != 0 {
		t.Fatalf("Status() = %#v", status)
	}
}

func TestTraeAddPreservesUnknownFieldsAndBacksUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{
  "future": {"nested": [1, true]},
  "mcpServers": {
    "external": {
      "command": "external-command",
      "args": ["--keep"],
      "env": {"SECRET": "external-secret"},
      "headers": {"Authorization": "Bearer external-header"},
      "futureField": 7
    }
  }
}`)
	mustWriteFile(t, path, original)
	adapter := NewTrae(path)
	profile := config.Profile{Name: "dev", Provider: "do-not-write", BaseURL: "https://example.invalid", Model: "do-not-write"}
	server, err := mcp.Custom("custom", "custom-mcp", []string{"--safe", "value"}, "dev")
	if err != nil {
		t.Fatal(err)
	}
	change, err := adapter.AddMCP(profile, server)
	if err != nil {
		t.Fatalf("AddMCP() error = %v", err)
	}
	if !change.Changed || change.BackupPath == "" {
		t.Fatalf("AddMCP() change = %#v", change)
	}
	if backup := mustReadFile(t, change.BackupPath); !reflect.DeepEqual(backup, original) {
		t.Fatal("backup does not contain the original bytes")
	}
	data := mustReadFile(t, path)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, root["future"], json.RawMessage(`{"nested":[1,true]}`))
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(root["mcpServers"], &servers); err != nil {
		t.Fatal(err)
	}
	var originalRoot map[string]json.RawMessage
	if err := json.Unmarshal(original, &originalRoot); err != nil {
		t.Fatal(err)
	}
	var originalServers map[string]json.RawMessage
	if err := json.Unmarshal(originalRoot["mcpServers"], &originalServers); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, servers["external"], originalServers["external"])
	entry, err := decodeEntry(servers["custom"])
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"run", "--profile", "dev", "--", "custom-mcp", "--safe", "value"}
	if entry.Command != "akey" || entry.Type != "" || !reflect.DeepEqual(entry.Args, wantArgs) {
		t.Fatalf("new entry = %#v", entry)
	}
	for _, forbidden := range []string{"do-not-write", "baseURL", "base_url", "provider", "model", "api_key", "apiKey"} {
		if strings.Contains(string(servers["custom"]), forbidden) {
			t.Fatalf("new entry contains %q", forbidden)
		}
	}
	listed, err := adapter.ListMCP()
	if err != nil {
		t.Fatalf("ListMCP() error = %v", err)
	}
	listedJSON, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"external-secret", "external-header", "Authorization", "futureField", "env", "headers"} {
		if strings.Contains(string(listedJSON), sensitive) {
			t.Fatalf("ListMCP() leaked %q: %s", sensitive, listedJSON)
		}
	}
	status, err := adapter.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.MCPCount != 2 || !reflect.DeepEqual(status.Profiles, []string{"dev"}) {
		t.Fatalf("Status() = %#v", status)
	}
}

func TestAddConflictDoesNotOverwriteOrBackUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{"mcpServers":{"external":{"command":"other","args":[],"env":{"KEY":"keep"}}}}`)
	mustWriteFile(t, path, original)
	adapter := NewTrae(path)
	server, err := mcp.Custom("external", "replacement", nil, "dev")
	if err != nil {
		t.Fatal(err)
	}
	change, err := adapter.AddMCP(config.Profile{Name: "dev"}, server)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("AddMCP() error = %v, want ErrConflict", err)
	}
	if change.Changed || change.BackupPath != "" {
		t.Fatalf("conflict change = %#v", change)
	}
	if data := mustReadFile(t, path); !reflect.DeepEqual(data, original) {
		t.Fatal("conflicting add modified the file")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("conflict created extra files: %v", entries)
	}
}

func TestRemoveManagedPreservesOthersAndExternalRemoveFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{
  "unknown": "keep",
  "mcpServers": {
    "managed": {"command":"akey","args":["run","--profile","dev","--","memory-mcp"]},
    "external": {"command":"other","args":[],"env":{"KEY":"keep"}}
  }
}`)
	mustWriteFile(t, path, original)
	adapter := NewTrae(path)
	change, err := adapter.RemoveMCP("managed")
	if err != nil {
		t.Fatalf("RemoveMCP() error = %v", err)
	}
	if !change.Changed || change.BackupPath == "" {
		t.Fatalf("RemoveMCP() change = %#v", change)
	}
	if backup := mustReadFile(t, change.BackupPath); !reflect.DeepEqual(backup, original) {
		t.Fatal("remove backup differs from original")
	}
	var root map[string]json.RawMessage
	data := mustReadFile(t, path)
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if string(root["unknown"]) != `"keep"` {
		t.Fatalf("unknown field = %s", root["unknown"])
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(root["mcpServers"], &servers); err != nil {
		t.Fatal(err)
	}
	if _, exists := servers["managed"]; exists {
		t.Fatal("managed server was not removed")
	}
	if _, exists := servers["external"]; !exists {
		t.Fatal("external server was removed")
	}
	beforeConflict := data
	conflict, err := adapter.RemoveMCP("external")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("external RemoveMCP() error = %v", err)
	}
	if conflict.Changed || conflict.BackupPath != "" {
		t.Fatalf("external remove change = %#v", conflict)
	}
	if after := mustReadFile(t, path); !reflect.DeepEqual(after, beforeConflict) {
		t.Fatal("external conflict modified configuration")
	}
	missing, err := adapter.RemoveMCP("missing")
	if err != nil || missing.Changed || missing.BackupPath != "" {
		t.Fatalf("missing RemoveMCP() = %#v, %v", missing, err)
	}
}

func TestClaudeCodeAddsStdioType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.json")
	mustWriteFile(t, path, []byte(`{"other":{"keep":true},"mcpServers":{}}`))
	adapter := NewClaudeCode(path)
	server, err := mcp.Builtin("memory", "work")
	if err != nil {
		t.Fatal(err)
	}
	change, err := adapter.AddMCP(config.Profile{Name: "work", Provider: "hidden", BaseURL: "hidden", Model: "hidden"}, server)
	if err != nil {
		t.Fatalf("AddMCP() error = %v", err)
	}
	if !change.Changed || change.BackupPath == "" {
		t.Fatalf("AddMCP() change = %#v", change)
	}
	data := mustReadFile(t, path)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, root["other"], json.RawMessage(`{"keep":true}`))
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(root["mcpServers"], &servers); err != nil {
		t.Fatal(err)
	}
	entry, err := decodeEntry(servers["memory"])
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "stdio" || entry.Command != "akey" {
		t.Fatalf("Claude entry = %#v", entry)
	}
	for _, forbidden := range []string{"provider", "baseURL", "base_url", "model", "hidden", "api_key", "token"} {
		if strings.Contains(string(servers["memory"]), forbidden) {
			t.Fatalf("Claude entry contains %q", forbidden)
		}
	}
}

func TestConfigureMergesMissingMCPServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{"future":{"enabled":true}}`)
	mustWriteFile(t, path, original)
	adapter := NewTrae(path)
	change, err := adapter.Configure(config.Profile{Name: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if !change.Changed || change.BackupPath == "" {
		t.Fatalf("Configure() = %#v", change)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(mustReadFile(t, path), &root); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, root["future"], json.RawMessage(`{"enabled":true}`))
	if string(root["mcpServers"]) != "{}" {
		t.Fatalf("mcpServers = %s", root["mcpServers"])
	}
}

func TestMalformedConfigFailsWithoutModification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	original := []byte(`{"mcpServers":`)
	mustWriteFile(t, path, original)
	adapter := NewTrae(path)
	server, err := mcp.Builtin("memory", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.AddMCP(config.Profile{Name: "dev"}, server); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("AddMCP() error = %v, want ErrInvalidConfig", err)
	}
	if data := mustReadFile(t, path); !reflect.DeepEqual(data, original) {
		t.Fatal("malformed input was modified")
	}
}

func TestJSONAdaptersRenameOnlyMatchingManagedProfile(t *testing.T) {
	tests := []struct {
		name       string
		typeField  string
		newAdapter func(string) Adapter
	}{
		{name: "Trae", newAdapter: func(path string) Adapter { return NewTrae(path) }},
		{name: "Claude Code", typeField: `,"type":"stdio"`, newAdapter: func(path string) Adapter { return NewClaudeCode(path) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			original := []byte(`{
  "futureTop": {"keep": true},
  "mcpServers": {
    "old-server": {
      "command": "akey",
      "args": ["run", "--profile", "old", "--", "custom-mcp", "--flag", "keep"]` + test.typeField + `,
      "env": {"EXISTING": "fake-existing-value"},
      "headers": {"X-Keep": "keep-header"},
      "futureField": {"nested": [1, 2]}
    },
    "other-profile": {
      "command": "akey",
      "args": ["run", "--profile", "other", "--", "memory-mcp"]
    },
    "external": {
      "command": "other-command",
      "args": ["run", "--profile", "old", "--", "custom-mcp"],
      "futureField": 9
    },
    "malformed": {
      "command": "akey",
      "args": ["run", "--profile", "old", "custom-mcp"]
    }
  }
}`)
			mustWriteFile(t, path, original)
			adapter := test.newAdapter(path)
			changes, err := adapter.RenameProfile("old", config.Profile{
				Name: "new", Provider: "new-provider-must-not-be-written", BaseURL: "https://example.invalid", Model: "new-model",
			})
			if err != nil {
				t.Fatalf("RenameProfile() error = %v", err)
			}
			if len(changes) != 1 || !changes[0].Changed || changes[0].Action != ActionRenameProfile || changes[0].BackupPath == "" {
				t.Fatalf("RenameProfile() changes = %#v", changes)
			}
			if backup := mustReadFile(t, changes[0].BackupPath); !reflect.DeepEqual(backup, original) {
				t.Fatal("rename backup differs from original")
			}
			data := mustReadFile(t, path)
			var root map[string]json.RawMessage
			var originalRoot map[string]json.RawMessage
			if err := json.Unmarshal(data, &root); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(original, &originalRoot); err != nil {
				t.Fatal(err)
			}
			assertJSONEqual(t, root["futureTop"], originalRoot["futureTop"])
			var servers map[string]json.RawMessage
			var originalServers map[string]json.RawMessage
			if err := json.Unmarshal(root["mcpServers"], &servers); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(originalRoot["mcpServers"], &originalServers); err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			var originalFields map[string]json.RawMessage
			if err := json.Unmarshal(servers["old-server"], &fields); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(originalServers["old-server"], &originalFields); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"command", "type", "env", "headers", "futureField"} {
				if originalFields[key] != nil {
					assertJSONEqual(t, fields[key], originalFields[key])
				}
			}
			var args []string
			if err := json.Unmarshal(fields["args"], &args); err != nil {
				t.Fatal(err)
			}
			wantArgs := []string{"run", "--profile", "new", "--", "custom-mcp", "--flag", "keep"}
			if !reflect.DeepEqual(args, wantArgs) {
				t.Fatalf("renamed args = %#v, want %#v", args, wantArgs)
			}
			for _, name := range []string{"other-profile", "external", "malformed"} {
				assertJSONEqual(t, servers[name], originalServers[name])
			}
			for _, forbidden := range []string{"new-provider-must-not-be-written", "new-model", "baseURL", "api_key", "apiKey"} {
				if strings.Contains(string(data), forbidden) {
					t.Fatalf("rename wrote forbidden value %q", forbidden)
				}
			}
			second, err := adapter.RenameProfile("old", config.Profile{Name: "new"})
			if err != nil || len(second) != 0 {
				t.Fatalf("second RenameProfile() = %#v, %v", second, err)
			}
			if after := mustReadFile(t, path); !reflect.DeepEqual(after, data) {
				t.Fatal("rename without matches modified the file")
			}
		})
	}
}

func TestRenameProfileWithoutConfigDoesNotCreateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "mcp.json")
	adapter := NewTrae(path)
	changes, err := adapter.RenameProfile("old", config.Profile{Name: "new"})
	if err != nil || len(changes) != 0 {
		t.Fatalf("RenameProfile() = %#v, %v", changes, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing config stat error = %v", err)
	}
}

func TestRegistryRenameProfileAggregatesChanges(t *testing.T) {
	registry := NewRegistry()
	paths := map[string]string{
		"trae":        filepath.Join(t.TempDir(), "trae.json"),
		"claude-code": filepath.Join(t.TempDir(), "claude.json"),
	}
	for name, path := range paths {
		mustWriteFile(t, path, []byte(`{"mcpServers":{"managed":{"command":"akey","args":["run","--profile","old","--","memory-mcp"]}}}`))
		var adapter Adapter
		if name == "trae" {
			adapter = NewTrae(path)
		} else {
			adapter = NewClaudeCode(path)
		}
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
	}
	changes, err := registry.RenameProfile("old", config.Profile{Name: "new"})
	if err != nil {
		t.Fatalf("Registry.RenameProfile() error = %v", err)
	}
	if len(changes) != 2 || changes[0].Agent != "claude-code" || changes[1].Agent != "trae" {
		t.Fatalf("Registry.RenameProfile() changes = %#v", changes)
	}
	for name := range paths {
		document, err := registry.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		listed, err := document.ListMCP()
		if err != nil {
			t.Fatal(err)
		}
		profile, managed := mcp.ManagedProfile(listed.MCPServers["managed"])
		if !managed || profile != "new" {
			t.Fatalf("%s entry = %#v", name, listed.MCPServers["managed"])
		}
	}
}

func TestDefaultRegistryContainsAdapters(t *testing.T) {
	registry := NewDefaultRegistry()
	adapters := registry.List()
	names := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		names = append(names, adapter.Name())
	}
	want := []string{"claude-code", "codex", "trae"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("registered names = %v, want %v", names, want)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertJSONEqual(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	var gotValue any
	var wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}
