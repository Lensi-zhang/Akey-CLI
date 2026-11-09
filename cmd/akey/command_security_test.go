package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lensi-zhang/Akey-CLI/internal/agent"
	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	keyringstore "github.com/Lensi-zhang/Akey-CLI/internal/keyring"
)

const testSecret = "fake-secret-not-a-real-key"

type fakeSecretStore struct {
	secrets   map[string]string
	getErr    error
	setErr    error
	deleteErr error
	statusErr error
	onDelete  func()
}

func newFakeSecretStore() *fakeSecretStore {
	return &fakeSecretStore{secrets: make(map[string]string)}
}

func (store *fakeSecretStore) Set(profile config.Profile, secret string) error {
	if store.setErr != nil {
		return store.setErr
	}
	store.secrets[profile.ID] = secret
	return nil
}

func (store *fakeSecretStore) Get(profile config.Profile) (string, error) {
	if store.getErr != nil {
		return "", store.getErr
	}
	secret, ok := store.secrets[profile.ID]
	if !ok {
		return "", apperr.NewAuth("not_configured", "Key 未配置", "请先登录", keyringstore.ErrNotConfigured)
	}
	return secret, nil
}

func (store *fakeSecretStore) Delete(profile config.Profile) error {
	if store.deleteErr != nil {
		return store.deleteErr
	}
	delete(store.secrets, profile.ID)
	if store.onDelete != nil {
		store.onDelete()
	}
	return nil
}

func (store *fakeSecretStore) Status(profile config.Profile) (keyringstore.Status, error) {
	status := keyringstore.Status{Backend: "fake"}
	if store.statusErr != nil {
		return status, store.statusErr
	}
	_, status.Configured = store.secrets[profile.ID]
	return status, nil
}

func newTestApplication(t *testing.T) (*application, *fakeSecretStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".akey", "profiles.json")
	secrets := newFakeSecretStore()
	application := &application{
		config:  config.NewStore(path),
		secrets: secrets,
		agents:  agent.NewRegistry(),
		openURL: func(string) error { return nil },
	}
	application.refreshPrinter()
	return application, secrets, path
}

func runTestCommand(application *application, input string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	application.stdin = strings.NewReader(input)
	application.stdout = &stdout
	application.stderr = &stderr
	application.refreshPrinter()
	root := newRootCommand(application)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		err = normalizeExecuteError(err)
		application.renderError(err)
		return errorExitCode(err), stdout.String(), stderr.String()
	}
	return 0, stdout.String(), stderr.String()
}

func createTestProfile(t *testing.T, application *application, name string) config.Profile {
	t.Helper()
	profile, err := application.config.Create(config.Profile{
		Name:      name,
		Provider:  "fake-provider",
		BaseURL:   "https://example.invalid/v1",
		Model:     "fake-model",
		EnvPrefix: "API",
	})
	if err != nil {
		t.Fatalf("创建测试 Profile 失败: %v", err)
	}
	return profile
}

func TestProfileCreateNonInteractiveJSON(t *testing.T) {
	application, _, path := newTestApplication(t)
	code, stdout, stderr := runTestCommand(application, "",
		"--non-interactive", "--output", "json", "profile", "create", "work",
		"--provider", "fake-provider",
		"--base-url", "https://example.invalid/v1",
		"--model", "fake-model",
		"--note", "temporary",
	)
	if code != 0 {
		t.Fatalf("profile create 退出码 = %d，stderr = %s", code, stderr)
	}
	var view profileView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("输出不是单个有效 JSON 文档: %v\n%s", err, stdout)
	}
	if view.Name != "work" || len(view.ID) != 32 || view.Key.Status != keyUnconfigured {
		t.Fatalf("Profile 输出不符合预期: %+v", view)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), testSecret) {
		t.Fatal("元数据文件包含伪密钥")
	}
}

func TestProfileAndAuthOutputsNeverContainSecret(t *testing.T) {
	application, secrets, _ := newTestApplication(t)
	profile := createTestProfile(t, application, "work")
	if err := secrets.Set(profile, testSecret); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"profile", "list", "--output", "json"},
		{"profile", "show", "work", "--output", "json"},
		{"auth", "status", "--profile", "work", "--output", "json"},
	} {
		code, stdout, stderr := runTestCommand(application, "", args...)
		if code != 0 {
			t.Fatalf("%v 退出码 = %d，stderr = %s", args, code, stderr)
		}
		if strings.Contains(stdout, testSecret) || strings.Contains(stderr, testSecret) {
			t.Fatalf("%v 输出了伪密钥", args)
		}
	}

	code, stdout, stderr := runTestCommand(application, "", "auth", "logout", "--profile", "work", "--yes", "--output", "json")
	if code != 0 {
		t.Fatalf("auth logout 退出码 = %d，stderr = %s", code, stderr)
	}
	if strings.Contains(stdout, testSecret) || strings.Contains(stderr, testSecret) {
		t.Fatal("auth logout 输出了伪密钥")
	}
	if _, err := application.config.Get(profile.Name); err != nil {
		t.Fatalf("logout 不应删除 Profile: %v", err)
	}
	if _, err := secrets.Get(profile); !errors.Is(err, keyringstore.ErrNotConfigured) {
		t.Fatalf("logout 后 Key 应不存在，err = %v", err)
	}
}

func TestProfileDeleteRestoresSecretWhenMetadataCommitFails(t *testing.T) {
	application, secrets, path := newTestApplication(t)
	profile := createTestProfile(t, application, "work")
	if err := secrets.Set(profile, testSecret); err != nil {
		t.Fatal(err)
	}
	secrets.onDelete = func() {
		_ = os.WriteFile(path, []byte("{invalid"), 0o600)
	}
	code, stdout, stderr := runTestCommand(application, "", "profile", "delete", "work", "--yes", "--output", "json")
	if code != 6 {
		t.Fatalf("删除补偿失败时退出码 = %d，期望 6；stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, testSecret) || strings.Contains(stderr, testSecret) {
		t.Fatal("删除补偿错误输出了伪密钥")
	}
	restored, err := secrets.Get(profile)
	if err != nil || restored != testSecret {
		t.Fatalf("元数据提交失败后未恢复 Key: err = %v", err)
	}
}

func TestUnknownCommandIsSafeUsageError(t *testing.T) {
	application, _, _ := newTestApplication(t)
	code, stdout, stderr := runTestCommand(application, "", testSecret)
	if code != 2 {
		t.Fatalf("未知命令退出码 = %d，期望 2", code)
	}
	if strings.Contains(stdout, testSecret) || strings.Contains(stderr, testSecret) {
		t.Fatal("未知命令错误回显了可能敏感的输入")
	}
}

func TestVersionJSONIsSingleDocument(t *testing.T) {
	application, _, _ := newTestApplication(t)
	code, stdout, stderr := runTestCommand(application, "", "--version", "--output", "json")
	if code != 0 {
		t.Fatalf("version 退出码 = %d，stderr = %s", code, stderr)
	}
	var value map[string]string
	if err := json.Unmarshal([]byte(stdout), &value); err != nil {
		t.Fatalf("version JSON 无效: %v\n%s", err, stdout)
	}
	if value["version"] == "" || value["commit"] == "" || value["built"] == "" {
		t.Fatalf("version JSON 缺少字段: %+v", value)
	}
}

func TestProfileRenameSyncsAgentsAndPreservesKey(t *testing.T) {
	application, secrets, _ := newTestApplication(t)
	profile := createTestProfile(t, application, "work")
	if err := secrets.Set(profile, testSecret); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{"managed":{"command":"akey","args":["run","--profile","work","--","memory-mcp"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := application.agents.Register(agent.NewTrae(configPath)); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTestCommand(application, "", "profile", "rename", "work", "production", "--output", "json")
	if code != 0 {
		t.Fatalf("profile rename 退出码 = %d，stderr = %s", code, stderr)
	}
	renamed, err := application.config.Get("production")
	if err != nil || renamed.ID != profile.ID {
		t.Fatalf("重命名后的 Profile = %+v, err = %v", renamed, err)
	}
	secret, err := secrets.Get(renamed)
	if err != nil || secret != testSecret {
		t.Fatalf("重命名后 Key 不可读取: err = %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compact.String(), `"--profile","production"`) || strings.Contains(compact.String(), `"--profile","work"`) {
		t.Fatalf("Agent 引用未同步: %s", data)
	}
	if strings.Contains(stdout, testSecret) || strings.Contains(stderr, testSecret) {
		t.Fatal("重命名输出了伪密钥")
	}
}

func TestProfileRenameReportsPartialAgentChanges(t *testing.T) {
	application, _, _ := newTestApplication(t)
	createTestProfile(t, application, "work")
	claudePath := filepath.Join(t.TempDir(), "claude.json")
	if err := os.WriteFile(claudePath, []byte(`{"mcpServers":{"managed":{"command":"akey","args":["run","--profile","work","--","memory-mcp"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	traePath := filepath.Join(t.TempDir(), "trae.json")
	if err := os.WriteFile(traePath, []byte(`{"mcpServers":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := application.agents.Register(agent.NewClaudeCode(claudePath)); err != nil {
		t.Fatal(err)
	}
	if err := application.agents.Register(agent.NewTrae(traePath)); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTestCommand(application, "", "profile", "rename", "work", "production", "--output", "json")
	if code != 1 {
		t.Fatalf("部分同步失败退出码 = %d，stdout = %s，stderr = %s", code, stdout, stderr)
	}
	var response errorResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("错误输出不是 JSON: %v\n%s", err, stdout)
	}
	if response.Error.Code != "agent_profile_rename_failed" || !strings.Contains(response.Error.Hint, "已完成 1 项") || !strings.Contains(response.Error.Hint, "未执行跨 Agent 回滚") {
		t.Fatalf("部分同步错误信息不完整: %+v", response.Error)
	}
	if _, err := application.config.Get("production"); err != nil {
		t.Fatalf("同步失败不应回滚 Profile 元数据: %v", err)
	}
	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compact.String(), `"--profile","production"`) {
		t.Fatalf("已完成的 Agent 变更被误回滚: %s", data)
	}
}

func TestConsecutivePromptsPreserveInput(t *testing.T) {
	input := strings.NewReader("first\nsecond\nthird\n")
	var output bytes.Buffer
	for _, expected := range []string{"first", "second", "third"} {
		actual, err := promptLine(input, &output, "Value: ")
		if err != nil {
			t.Fatal(err)
		}
		if actual != expected {
			t.Fatalf("连续输入 = %q，期望 %q", actual, expected)
		}
	}
}
