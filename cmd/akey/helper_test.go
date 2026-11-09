package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Lensi-zhang/Akey-CLI/internal/agent"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/mcp"
)

type fakeHelperAgent struct {
	name           string
	detected       bool
	changed        bool
	configureCalls int
	profiles       []config.Profile
}

func (fake *fakeHelperAgent) Name() string { return fake.name }

func (fake *fakeHelperAgent) Detect() (agent.InstallInfo, error) {
	return agent.InstallInfo{
		Name:         fake.name,
		Installed:    fake.detected,
		ConfigExists: fake.detected,
		ConfigPath:   "fake-config.json",
	}, nil
}

func (fake *fakeHelperAgent) Status() (agent.Status, error) {
	return agent.Status{Name: fake.name}, nil
}

func (fake *fakeHelperAgent) Configure(profile config.Profile) (agent.ChangeSet, error) {
	fake.configureCalls++
	fake.profiles = append(fake.profiles, profile)
	return agent.ChangeSet{Agent: fake.name, Action: agent.ActionConfigure, Changed: fake.changed}, nil
}

func (fake *fakeHelperAgent) AddMCP(config.Profile, mcp.Server) (agent.ChangeSet, error) {
	return agent.ChangeSet{}, nil
}

func (fake *fakeHelperAgent) RemoveMCP(string) (agent.ChangeSet, error) {
	return agent.ChangeSet{}, nil
}

func (fake *fakeHelperAgent) ListMCP() (mcp.Document, error) {
	return mcp.EmptyDocument(), nil
}

func (fake *fakeHelperAgent) RenameProfile(string, config.Profile) ([]agent.ChangeSet, error) {
	return nil, nil
}

type helperScriptReader struct {
	remaining       string
	secretSubmitted bool
}

func (reader *helperScriptReader) Read(buffer []byte) (int, error) {
	if reader.remaining == "" {
		return 0, io.EOF
	}
	if len(buffer) == 1 {
		reader.secretSubmitted = false
		buffer[0] = reader.remaining[0]
		reader.remaining = reader.remaining[1:]
		return 1, nil
	}
	if reader.secretSubmitted {
		return 0, io.EOF
	}
	line, rest, found := strings.Cut(reader.remaining, "\n")
	reader.remaining = rest
	if !found {
		reader.remaining = ""
	}
	// Bubble Tea 需要 CR 提交，且一次只能读取一行，避免吞掉后续 Agent 回答。
	line = strings.TrimSuffix(line, "\r") + "\r"
	if len(line) > len(buffer) {
		return 0, io.ErrShortBuffer
	}
	reader.secretSubmitted = true
	return copy(buffer, line), nil
}

func runInteractiveHelperCommand(application *application, input string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	application.stdin = &helperScriptReader{remaining: input}
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

func runHelperJSON(t *testing.T, application *application, input string, explicit bool) (helperView, string, string) {
	t.Helper()
	args := []string{"--output", "json"}
	if explicit {
		args = append(args, "helper")
	}
	code, stdout, stderr := runInteractiveHelperCommand(application, input, args...)
	if code != 0 {
		t.Fatalf("helper 退出码 = %d，stdout = %s，stderr = %s", code, stdout, stderr)
	}
	var view helperView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("helper 输出不是单个有效 JSON 文档: %v\n%s", err, stdout)
	}
	if !strings.Contains(stderr, "请选择服务商与 Key 类型") {
		t.Fatalf("stderr 缺少服务商提示: %s", stderr)
	}
	for _, prompt := range []string{"请选择服务商", "Model 或 Endpoint", "确认修改", "检测到多个 Agent"} {
		if strings.Contains(stdout, prompt) {
			t.Fatalf("交互提示写入 stdout: %q\n%s", prompt, stdout)
		}
	}
	if strings.Contains(stdout, testSecret) || strings.Contains(stderr, testSecret) {
		t.Fatal("helper 输出了伪密钥")
	}
	return view, stdout, stderr
}

func TestDefaultHelperEntrypoints(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "无参数"
		if explicit {
			name = "helper"
		}
		t.Run(name, func(t *testing.T) {
			application, secrets, _ := newTestApplication(t)
			view, _, _ := runHelperJSON(t, application, "\nmodel\n"+testSecret+"\n", explicit)
			if view.Profile != "volcengine" || view.CredentialType != config.CredentialAPI || !view.Current || view.AgentStatus != "not_detected" {
				t.Fatalf("helper 结果不符合预期: %+v", view)
			}
			profile, err := application.config.Get("volcengine")
			if err != nil {
				t.Fatal(err)
			}
			if secrets.secrets[profile.ID] != testSecret {
				t.Fatal("Key 未按新建 Profile.ID 保存")
			}
		})
	}

	for _, args := range [][]string{{"--help"}, {"--version", "--output", "json"}} {
		t.Run("不进入向导 "+strings.Join(args, " "), func(t *testing.T) {
			application, secrets, _ := newTestApplication(t)
			code, stdout, stderr := runTestCommand(application, "", args...)
			if code != 0 {
				t.Fatalf("%v 退出码 = %d，stderr = %s", args, code, stderr)
			}
			if strings.Contains(stdout+stderr, "请选择服务商与 Key 类型") || len(secrets.secrets) != 0 {
				t.Fatalf("%v 意外进入向导", args)
			}
			profiles, err := application.config.List()
			if err != nil || len(profiles) != 0 {
				t.Fatalf("%v 创建了 Profile: %+v, err = %v", args, profiles, err)
			}
		})
	}

	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "noninteractive 无参数", true: "noninteractive helper"}[explicit], func(t *testing.T) {
			application, secrets, _ := newTestApplication(t)
			args := []string{"--non-interactive", "--output", "json"}
			if explicit {
				args = append(args, "helper")
			}
			code, stdout, stderr := runTestCommand(application, "", args...)
			if code != 2 {
				t.Fatalf("noninteractive 退出码 = %d，期望 2；stdout = %s，stderr = %s", code, stdout, stderr)
			}
			var response errorResponse
			if err := json.Unmarshal([]byte(stdout), &response); err != nil || response.Error.Code != "interactive_required" {
				t.Fatalf("noninteractive 错误不符合预期: %+v, err = %v", response, err)
			}
			if strings.Contains(stdout+stderr, "请选择服务商与 Key 类型") || len(secrets.secrets) != 0 {
				t.Fatal("noninteractive 模式进入了交互或保存了 Key")
			}
		})
	}
}

func TestHelperProviderBranches(t *testing.T) {
	tests := []struct {
		name         string
		choice       string
		extraInput   string
		profileName  string
		provider     string
		baseURL      string
		credential   config.CredentialType
		nameConflict bool
	}{
		{name: "普通", choice: "1", profileName: "volcengine", provider: "volcengine", baseURL: "https://ark.cn-beijing.volces.com/api/v3", credential: config.CredentialAPI},
		{name: "Agent Plan", choice: "2", profileName: "volcengine-agent-plan", provider: "volcengine", baseURL: "https://ark.cn-beijing.volces.com/api/plan/v3", credential: config.CredentialAgentPlan},
		{name: "Coding Plan", choice: "3", profileName: "volcengine-coding-plan", provider: "volcengine", baseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", credential: config.CredentialCodingPlan},
		{name: "Custom", choice: "8", extraInput: "https://custom.example.com/v1\n", profileName: "custom", provider: "custom", baseURL: "https://custom.example.com/v1", credential: config.CredentialAPI},
		{name: "名称冲突", choice: "", profileName: "volcengine-2", provider: "volcengine", baseURL: "https://ark.cn-beijing.volces.com/api/v3", credential: config.CredentialAPI, nameConflict: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application, secrets, _ := newTestApplication(t)
			if test.nameConflict {
				createTestProfile(t, application, "volcengine")
			}
			input := test.choice + "\n" + test.extraInput + "model\n" + testSecret + "\n"
			view, _, _ := runHelperJSON(t, application, input, true)
			if view.Profile != test.profileName || view.CredentialType != test.credential || !view.Current || view.AgentStatus != "not_detected" {
				t.Fatalf("helper 结果不符合预期: %+v", view)
			}
			profile, err := application.config.Get(test.profileName)
			if err != nil {
				t.Fatal(err)
			}
			if profile.Provider != test.provider || profile.BaseURL != test.baseURL || profile.Model != "model" || profile.CredentialType != test.credential || profile.EnvPrefix != "API" {
				t.Fatalf("Profile 元数据不符合预设: %+v", profile)
			}
			if secrets.secrets[profile.ID] != testSecret {
				t.Fatal("Key 未按 Profile.ID 保存")
			}
		})
	}
}

func TestHelperAgentConfiguration(t *testing.T) {
	type agentSpec struct {
		name     string
		detected bool
		changed  bool
	}
	tests := []struct {
		name           string
		agents         []agentSpec
		extraInput     string
		wantAgent      string
		wantStatus     string
		wantConfigured bool
		wantCalls      map[string]int
		wantSelection  bool
		wantConfirm    bool
	}{
		{name: "零 Agent", wantStatus: "not_detected"},
		{name: "未安装 Agent", agents: []agentSpec{{name: "absent"}}, wantStatus: "not_detected"},
		{name: "一个 changed", agents: []agentSpec{{name: "solo", detected: true, changed: true}}, extraInput: "y\n", wantAgent: "solo", wantStatus: "configured", wantConfigured: true, wantCalls: map[string]int{"solo": 1}, wantConfirm: true},
		{name: "一个 unchanged", agents: []agentSpec{{name: "solo", detected: true}}, extraInput: "yes\n", wantAgent: "solo", wantStatus: "unchanged", wantConfigured: true, wantCalls: map[string]int{"solo": 1}, wantConfirm: true},
		{name: "默认拒绝", agents: []agentSpec{{name: "solo", detected: true}}, extraInput: "\n", wantAgent: "solo", wantStatus: "skipped", wantConfirm: true},
		{name: "多个选择第二个", agents: []agentSpec{{name: "beta", detected: true, changed: true}, {name: "alpha", detected: true}}, extraInput: "2\ny\n", wantAgent: "beta", wantStatus: "configured", wantConfigured: true, wantCalls: map[string]int{"beta": 1}, wantSelection: true, wantConfirm: true},
		{name: "多个零跳过", agents: []agentSpec{{name: "alpha", detected: true}, {name: "beta", detected: true}}, extraInput: "0\n", wantStatus: "skipped", wantSelection: true},
		{name: "Codex 无需配置", agents: []agentSpec{{name: "codex", detected: true}}, wantAgent: "codex", wantStatus: "not_required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application, _, _ := newTestApplication(t)
			fakes := make(map[string]*fakeHelperAgent, len(test.agents))
			for _, spec := range test.agents {
				fake := &fakeHelperAgent{name: spec.name, detected: spec.detected, changed: spec.changed}
				fakes[spec.name] = fake
				if err := application.agents.Register(fake); err != nil {
					t.Fatal(err)
				}
			}
			view, _, stderr := runHelperJSON(t, application, "\nmodel\n"+testSecret+"\n"+test.extraInput, true)
			if view.Agent != test.wantAgent || view.AgentStatus != test.wantStatus || view.Configured != test.wantConfigured {
				t.Fatalf("Agent 结果 = %+v，期望 agent=%q status=%q configured=%v", view, test.wantAgent, test.wantStatus, test.wantConfigured)
			}
			for name, fake := range fakes {
				if fake.configureCalls != test.wantCalls[name] {
					t.Fatalf("Agent %s Configure 调用 = %d，期望 %d", name, fake.configureCalls, test.wantCalls[name])
				}
				if fake.configureCalls == 1 && (len(fake.profiles) != 1 || fake.profiles[0].Name != view.Profile) {
					t.Fatalf("Agent %s 收到错误 Profile: %+v", name, fake.profiles)
				}
			}
			if strings.Contains(stderr, "检测到多个 Agent") != test.wantSelection {
				t.Fatalf("多 Agent 选择提示不符合预期: %s", stderr)
			}
			if strings.Contains(stderr, "确认修改上述 Agent 配置？") != test.wantConfirm {
				t.Fatalf("Agent 确认提示不符合预期: %s", stderr)
			}
		})
	}
}

func TestHelperKeySetFailureRollsBackProfile(t *testing.T) {
	application, secrets, path := newTestApplication(t)
	secrets.setErr = errors.New("fake set failure")
	fake := &fakeHelperAgent{name: "solo", detected: true}
	if err := application.agents.Register(fake); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runInteractiveHelperCommand(application, "\nmodel\n"+testSecret+"\n", "--output", "json", "helper")
	if code != 3 {
		t.Fatalf("Key Set 失败退出码 = %d，期望 3；stdout = %s，stderr = %s", code, stdout, stderr)
	}
	var response errorResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil || response.Error.Code != "helper_key_save_failed" {
		t.Fatalf("Key Set 错误不符合预期: %+v, err = %v", response, err)
	}
	profiles, err := application.config.List()
	if err != nil || len(profiles) != 0 {
		t.Fatalf("Set 失败后 Profile 未回滚: %+v, err = %v", profiles, err)
	}
	if len(secrets.secrets) != 0 || fake.configureCalls != 0 {
		t.Fatal("Set 失败后残留 Key 或修改了 Agent")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, testSecret) || strings.Contains(stderr, testSecret) || strings.Contains(string(data), testSecret) {
		t.Fatal("Set 失败路径泄露了伪密钥")
	}
}

func TestHelperConsecutiveProfilesIsolateSecrets(t *testing.T) {
	application, secrets, path := newTestApplication(t)
	ordinarySecret := testSecret + "-ordinary"
	planSecret := testSecret + "-plan"
	_, ordinaryOut, ordinaryErr := runHelperJSON(t, application, "1\nordinary-model\n"+ordinarySecret+"\n", false)
	_, planOut, planErr := runHelperJSON(t, application, "2\nplan-model\n"+planSecret+"\n", true)

	ordinary, err := application.config.Get("volcengine")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := application.config.Get("volcengine-agent-plan")
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.ID == plan.ID || ordinary.ID == "" || plan.ID == "" {
		t.Fatalf("连续 Profile.ID 未隔离: %q, %q", ordinary.ID, plan.ID)
	}
	if len(secrets.secrets) != 2 || secrets.secrets[ordinary.ID] != ordinarySecret || secrets.secrets[plan.ID] != planSecret {
		t.Fatalf("secrets 未按不同 Profile.ID 保存: %+v", secrets.secrets)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	combined := ordinaryOut + ordinaryErr + planOut + planErr + string(data)
	if strings.Contains(combined, ordinarySecret) || strings.Contains(combined, planSecret) {
		t.Fatal("stdout、stderr 或 profiles.json 泄露了伪密钥")
	}
}
