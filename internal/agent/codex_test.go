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

type runnerCall struct {
	path string
	args []string
}

type runnerResponse struct {
	output []byte
	err    error
}

type fakeCommandRunner struct {
	path      string
	lookErr   error
	responses map[string]runnerResponse
	calls     []runnerCall
}

func (f *fakeCommandRunner) LookPath(file string) (string, error) {
	if file != "codex" {
		return "", errors.New("unexpected executable")
	}
	return f.path, f.lookErr
}

func (f *fakeCommandRunner) Output(path string, args ...string) ([]byte, error) {
	copiedArgs := append([]string(nil), args...)
	f.calls = append(f.calls, runnerCall{path: path, args: copiedArgs})
	response, exists := f.responses[strings.Join(args, "\x00")]
	if !exists {
		return nil, errors.New("unexpected fake command")
	}
	return append([]byte(nil), response.output...), response.err
}

func TestCodexAddUsesOfficialCommandAndInjectedPath(t *testing.T) {
	runner := &fakeCommandRunner{
		path:      "/fake/tools/codex",
		responses: map[string]runnerResponse{},
	}
	addArgs := []string{"mcp", "add", "custom", "--", "akey", "run", "--profile", "dev", "--", "custom-mcp", "--flag", "safe value"}
	runner.responses[strings.Join(addArgs, "\x00")] = runnerResponse{output: []byte(`{"ok":true}`)}
	adapter := NewCodex(runner)
	server, err := mcp.Custom("custom", "custom-mcp", []string{"--flag", "safe value"}, "dev")
	if err != nil {
		t.Fatal(err)
	}
	change, err := adapter.AddMCP(config.Profile{Name: "dev", Provider: "not-forwarded"}, server)
	if err != nil {
		t.Fatalf("AddMCP() error = %v", err)
	}
	if !change.Changed || change.ConfigPath != "" {
		t.Fatalf("AddMCP() change = %#v", change)
	}
	wantCalls := []runnerCall{{path: "/fake/tools/codex", args: addArgs}}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
	info, err := adapter.Detect()
	if err != nil || !info.Installed || info.ExecutablePath != "/fake/tools/codex" || info.ConfigPath != "" {
		t.Fatalf("Detect() = %#v, %v", info, err)
	}
}

func TestCodexListWhitelistsMaliciousSensitiveFields(t *testing.T) {
	output := []byte(`[
  {
    "name": "safe",
    "command": "akey",
    "args": ["run", "--profile", "dev", "--", "memory-mcp"],
    "env": {"API_KEY": "fake-env-secret"},
    "headers": {"Authorization": "Bearer fake-header-secret"},
    "url": "https://example.invalid/?token=fake-url-secret",
    "unknown": "do-not-forward"
  }
]`)
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json": {output: output},
	}}
	document, err := NewCodex(runner).ListMCP()
	if err != nil {
		t.Fatalf("ListMCP() error = %v", err)
	}
	entry := document.MCPServers["safe"]
	if entry.Command != "akey" || len(entry.Args) != 5 {
		t.Fatalf("entry = %#v", entry)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"fake-env-secret", "fake-header-secret", "fake-url-secret", "Authorization", "headers", "env", "unknown", "url"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("ListMCP() forwarded %q: %s", forbidden, encoded)
		}
	}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0].args, []string{"mcp", "list", "--json"}) {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestCodexListSupportsMCPServersObject(t *testing.T) {
	output := []byte(`{"mcpServers":{"other":{"command":"other-command","args":[],"env":{"KEY":"not-forwarded"}}}}`)
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json": {output: output},
	}}
	document, err := NewCodex(runner).ListMCP()
	if err != nil {
		t.Fatal(err)
	}
	if len(document.MCPServers) != 1 || document.MCPServers["other"].Command != "other-command" {
		t.Fatalf("document = %#v", document)
	}
}

func TestCodexRemoveChecksOwnershipAndUsesJSONGet(t *testing.T) {
	getOutput := []byte(`{
  "name": "managed",
  "config": {
    "command": "akey",
    "args": ["run", "--profile", "dev", "--", "memory-mcp"],
    "env": {"TOKEN": "not-forwarded"}
  }
}`)
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00get\x00managed\x00--json": {output: getOutput},
		"mcp\x00remove\x00managed":        {output: []byte(`{}`)},
	}}
	change, err := NewCodex(runner).RemoveMCP("managed")
	if err != nil {
		t.Fatalf("RemoveMCP() error = %v", err)
	}
	if !change.Changed {
		t.Fatalf("RemoveMCP() = %#v", change)
	}
	wantCalls := []runnerCall{
		{path: "/fake/codex", args: []string{"mcp", "get", "managed", "--json"}},
		{path: "/fake/codex", args: []string{"mcp", "remove", "managed"}},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
}

func TestCodexRemoveRefusesExternalServer(t *testing.T) {
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00get\x00external\x00--json": {output: []byte(`{"name":"external","command":"other","args":[],"headers":{"Authorization":"fake-header"}}`)},
	}}
	change, err := NewCodex(runner).RemoveMCP("external")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("RemoveMCP() error = %v", err)
	}
	if change.Changed || len(runner.calls) != 1 {
		t.Fatalf("change = %#v, calls = %#v", change, runner.calls)
	}
}

func TestCodexRejectsMalformedOutputWithoutEcho(t *testing.T) {
	const marker = "fake-malformed-output-secret"
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json": {output: []byte(`not-json ` + marker)},
	}}
	_, err := NewCodex(runner).ListMCP()
	if !errors.Is(err, ErrCodexOutput) {
		t.Fatalf("ListMCP() error = %v, want ErrCodexOutput", err)
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("error echoed malicious output: %v", err)
	}
}

func TestCodexCommandErrorDoesNotEchoRunnerError(t *testing.T) {
	const marker = "fake-runner-error-secret"
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json": {err: errors.New(marker)},
	}}
	_, err := NewCodex(runner).ListMCP()
	if !errors.Is(err, ErrCodexCommand) {
		t.Fatalf("ListMCP() error = %v, want ErrCodexCommand", err)
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("error echoed runner error: %v", err)
	}
}

func TestCodexRenameProfileUsesRemoveAddForManagedOldEntriesOnly(t *testing.T) {
	listOutput := []byte(`[
  {"name":"managed","command":"akey","args":["run","--profile","old","--","custom-mcp","--flag","keep"]},
  {"name":"other-profile","command":"akey","args":["run","--profile","other","--","memory-mcp"]},
  {"name":"external","command":"other-command","args":["run","--profile","old","--","custom-mcp"]},
  {"name":"malformed","command":"akey","args":["run","--profile","old","custom-mcp"]}
]`)
	getOutput := []byte(`{"name":"managed","config":{"command":"akey","args":["run","--profile","old","--","custom-mcp","--flag","keep"],"type":"stdio"}}`)
	addArgs := []string{"mcp", "add", "managed", "--", "akey", "run", "--profile", "new", "--", "custom-mcp", "--flag", "keep"}
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json":           {output: listOutput},
		"mcp\x00get\x00managed\x00--json": {output: getOutput},
		"mcp\x00remove\x00managed":        {output: []byte(`{}`)},
		strings.Join(addArgs, "\x00"):     {output: []byte(`{}`)},
	}}
	changes, err := NewCodex(runner).RenameProfile("old", config.Profile{Name: "new", Provider: "not-forwarded"})
	if err != nil {
		t.Fatalf("RenameProfile() error = %v", err)
	}
	if len(changes) != 2 || changes[0].Action != ActionRemove || changes[1].Action != ActionAdd {
		t.Fatalf("RenameProfile() changes = %#v", changes)
	}
	for _, change := range changes {
		if !change.Changed || change.ServerName != "managed" {
			t.Fatalf("change = %#v", change)
		}
	}
	wantCalls := []runnerCall{
		{path: "/fake/codex", args: []string{"mcp", "list", "--json"}},
		{path: "/fake/codex", args: []string{"mcp", "get", "managed", "--json"}},
		{path: "/fake/codex", args: []string{"mcp", "remove", "managed"}},
		{path: "/fake/codex", args: addArgs},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
	for _, call := range runner.calls {
		joined := strings.Join(call.args, " ")
		for _, forbidden := range []string{"not-forwarded", "TOKEN", "env"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("command forwarded %q: %s", forbidden, joined)
			}
		}
	}
}

func TestCodexRenameProfileReportsPartialFailureWithoutEcho(t *testing.T) {
	const marker = "fake-add-error-secret"
	listOutput := []byte(`[{"name":"managed","command":"akey","args":["run","--profile","old","--","memory-mcp"]}]`)
	getOutput := []byte(`{"name":"managed","command":"akey","args":["run","--profile","old","--","memory-mcp"]}`)
	addArgs := []string{"mcp", "add", "managed", "--", "akey", "run", "--profile", "new", "--", "memory-mcp"}
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json":           {output: listOutput},
		"mcp\x00get\x00managed\x00--json": {output: getOutput},
		"mcp\x00remove\x00managed":        {output: []byte(`{}`)},
		strings.Join(addArgs, "\x00"):     {err: errors.New(marker)},
	}}
	changes, err := NewCodex(runner).RenameProfile("old", config.Profile{Name: "new"})
	if !errors.Is(err, ErrCodexCommand) {
		t.Fatalf("RenameProfile() error = %v", err)
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("error echoed CLI failure: %v", err)
	}
	if len(changes) != 1 || changes[0].Action != ActionRemove || !changes[0].Changed {
		t.Fatalf("partial changes = %#v", changes)
	}
}

func TestCodexRenameProfileRejectsExtendedEntryBeforeRemove(t *testing.T) {
	const marker = "fake-extension-secret"
	listOutput := []byte(`[{"name":"managed","command":"akey","args":["run","--profile","old","--","memory-mcp"]}]`)
	getOutput := []byte(`{
  "name": "managed",
  "config": {
    "command": "akey",
    "args": ["run", "--profile", "old", "--", "memory-mcp"],
    "env": {"TOKEN": "fake-extension-secret"},
    "headers": {"Authorization": "Bearer fake-extension-secret"},
    "futureField": "do-not-drop"
  }
}`)
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json":           {output: listOutput},
		"mcp\x00get\x00managed\x00--json": {output: getOutput},
	}}
	changes, err := NewCodex(runner).RenameProfile("old", config.Profile{Name: "new"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("RenameProfile() error = %v, want ErrConflict", err)
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("error echoed extension data: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %#v, want none", changes)
	}
	wantCalls := []runnerCall{
		{path: "/fake/codex", args: []string{"mcp", "list", "--json"}},
		{path: "/fake/codex", args: []string{"mcp", "get", "managed", "--json"}},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want no remove/add and %#v", runner.calls, wantCalls)
	}
}

func TestCodexRenameProfileRechecksCurrentProfileBeforeRemove(t *testing.T) {
	listOutput := []byte(`[{"name":"managed","command":"akey","args":["run","--profile","old","--","memory-mcp"]}]`)
	getOutput := []byte(`{"name":"managed","command":"akey","args":["run","--profile","other","--","memory-mcp"]}`)
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json":           {output: listOutput},
		"mcp\x00get\x00managed\x00--json": {output: getOutput},
	}}
	changes, err := NewCodex(runner).RenameProfile("old", config.Profile{Name: "new"})
	if !errors.Is(err, ErrConflict) || len(changes) != 0 {
		t.Fatalf("RenameProfile() = %#v, %v; want no changes and ErrConflict", changes, err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %#v, want only list/get", runner.calls)
	}
}

func TestCodexRenameProfileWithoutMatchesOnlyLists(t *testing.T) {
	listOutput := []byte(`[{"name":"other","command":"akey","args":["run","--profile","other","--","memory-mcp"]}]`)
	runner := &fakeCommandRunner{path: "/fake/codex", responses: map[string]runnerResponse{
		"mcp\x00list\x00--json": {output: listOutput},
	}}
	changes, err := NewCodex(runner).RenameProfile("old", config.Profile{Name: "new"})
	if err != nil || len(changes) != 0 || len(runner.calls) != 1 {
		t.Fatalf("RenameProfile() = %#v, %v; calls = %#v", changes, err, runner.calls)
	}
}

func TestCodexRenameProfileWithoutCLIOrConfigIsNoop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	runner := &fakeCommandRunner{lookErr: errors.New("not installed"), responses: map[string]runnerResponse{}}
	changes, err := NewCodex(runner).RenameProfile("old", config.Profile{Name: "new"})
	if err != nil || len(changes) != 0 {
		t.Fatalf("RenameProfile() = %#v, %v; want no-op", changes, err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unexpected calls = %#v", runner.calls)
	}
}

func TestCodexRenameProfileWithConfigButWithoutCLIFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configDirectory := filepath.Join(home, ".codex")
	if err := os.Mkdir(configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "config.toml"), []byte("# must not be parsed"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{lookErr: errors.New("not installed"), responses: map[string]runnerResponse{}}
	changes, err := NewCodex(runner).RenameProfile("old", config.Profile{Name: "new"})
	if !errors.Is(err, ErrCodexUnavailable) || len(changes) != 0 {
		t.Fatalf("RenameProfile() = %#v, %v; want ErrCodexUnavailable", changes, err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unexpected calls = %#v", runner.calls)
	}
}

func TestCodexUnavailableDoesNotRunCommand(t *testing.T) {
	runner := &fakeCommandRunner{lookErr: errors.New("not installed"), responses: map[string]runnerResponse{}}
	adapter := NewCodex(runner)
	info, err := adapter.Detect()
	if err != nil || info.Installed {
		t.Fatalf("Detect() = %#v, %v", info, err)
	}
	if _, err := adapter.ListMCP(); !errors.Is(err, ErrCodexUnavailable) {
		t.Fatalf("ListMCP() error = %v, want ErrCodexUnavailable", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unexpected calls = %#v", runner.calls)
	}
}
