package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/agent"
	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/format"
	"github.com/Lensi-zhang/Akey-CLI/internal/validate"

	"github.com/spf13/cobra"
)

type helperView struct {
	Profile        string                `json:"profile"`
	CredentialType config.CredentialType `json:"credential_type"`
	Current        bool                  `json:"current"`
	Agent          string                `json:"agent,omitempty"`
	Configured     bool                  `json:"configured"`
	AgentStatus    string                `json:"agent_status"`
}

func newHelperCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "helper",
		Short: "交互式创建 Profile、保存 Key 并初始化 Agent",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return runDefaultHelper(application)
		},
	}
}

func runDefaultHelper(application *application) error {
	if err := application.requireInteractive(); err != nil {
		return err
	}
	return runHelper(application)
}

func runHelper(application *application) error {
	preset, err := promptProviderPreset(application)
	if err != nil {
		return err
	}
	baseURL := preset.BaseURL
	if preset.ID == "custom" {
		baseURL, err = promptLine(application.stdin, application.stderr, "Base URL: ")
		if err != nil {
			return err
		}
		if err := validate.BaseURL(baseURL); err != nil {
			return err
		}
	}
	model, err := promptLine(application.stdin, application.stderr, "Model 或 Endpoint ID（从服务商控制台复制）: ")
	if err != nil {
		return err
	}
	name, err := nextHelperProfileName(application, preset.ID)
	if err != nil {
		return err
	}

	secretLabel := "API Key: "
	switch preset.CredentialType {
	case config.CredentialAgentPlan:
		secretLabel = "Agent Plan Key: "
	case config.CredentialCodingPlan:
		secretLabel = "Coding Plan Key: "
	}
	secret, err := promptSecret(application.stdin, application.stderr, secretLabel)
	if err != nil {
		if errors.Is(err, errInputCanceled) {
			return apperr.NewCanceled("helper_canceled", "向导已取消", "未创建 Profile、Key 或 Agent 配置")
		}
		if errors.Is(err, errEmptySecret) {
			return apperr.NewUsage("empty_key", "Key 不能为空", "未创建任何配置，请重新运行 akey")
		}
		return apperr.NewRuntime("key_input_failed", "无法读取隐藏密钥输入", "未创建任何配置，请检查终端后重试", err)
	}
	created, err := application.config.Create(config.Profile{
		Name:           name,
		Provider:       preset.Provider,
		CredentialType: preset.CredentialType,
		BaseURL:        baseURL,
		Model:          model,
		EnvPrefix:      config.DefaultEnvPrefix,
	})
	if err != nil {
		return err
	}
	store, err := application.secretStore()
	if err == nil {
		err = store.Set(created, secret)
	}
	if err != nil {
		if rollbackErr := application.config.Delete(created.Name); rollbackErr != nil {
			return apperr.NewAuth("helper_rollback_failed", "Key 保存失败，且新建 Profile 回滚失败", "请检查配置文件与系统密钥环", errors.Join(err, rollbackErr))
		}
		return apperr.NewAuth("helper_key_save_failed", "Key 保存失败，已回滚新建 Profile", "未修改 Agent 配置，请检查系统密钥环后重试", err)
	}
	if err := application.config.Use(created.Name); err != nil {
		return apperr.NewConfig("helper_use_failed", "Profile 与 Key 已创建，但默认环境切换失败", fmt.Sprintf("请执行 akey profile use %s", created.Name), err)
	}
	application.printer.success(fmt.Sprintf("Profile %s（%s）与 Key 已创建，并设为默认环境", created.Name, created.CredentialType))

	view := helperView{Profile: created.Name, CredentialType: created.CredentialType, Current: true}
	if err := configureDetectedAgent(application, created, &view); err != nil {
		return err
	}
	return application.emit(view, format.Table([]string{"PROFILE", "TYPE", "CURRENT", "AGENT", "AGENT_STATUS"}, [][]string{{
		view.Profile,
		string(view.CredentialType),
		yesNo(view.Current),
		view.Agent,
		view.AgentStatus,
	}}))
}

func promptProviderPreset(application *application) (config.ProviderPreset, error) {
	presets := config.ProviderPresets()
	_, _ = fmt.Fprintln(application.stderr, "请选择服务商与 Key 类型：")
	for index, preset := range presets {
		_, _ = fmt.Fprintf(application.stderr, "  %d) %s\n", index+1, preset.Name)
	}
	choice, err := readLine(application.stdin, application.stderr, "请输入序号 [1]: ")
	if err != nil {
		return config.ProviderPreset{}, err
	}
	if choice == "" {
		choice = "1"
	}
	index, err := strconv.Atoi(choice)
	if err != nil || index < 1 || index > len(presets) {
		return config.ProviderPreset{}, apperr.NewUsage("invalid_provider_selection", "服务商序号无效", fmt.Sprintf("请输入 1 到 %d", len(presets)))
	}
	return presets[index-1], nil
}

func nextHelperProfileName(application *application, presetID string) (string, error) {
	profiles, err := application.config.List()
	if err != nil {
		return "", err
	}
	used := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		used[profile.Name] = true
	}
	if !used[presetID] {
		return presetID, nil
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", presetID, suffix)
		if !used[candidate] {
			return candidate, nil
		}
	}
}

func configureDetectedAgent(application *application, profile config.Profile, view *helperView) error {
	detected, err := detectAgents(application)
	if err != nil {
		return helperPartialError("Agent 检测失败", err)
	}
	candidates := make([]agent.InstallInfo, 0, len(detected))
	for _, info := range detected {
		if info.Installed || info.ConfigExists {
			candidates = append(candidates, info)
		}
	}
	if len(candidates) == 0 {
		view.AgentStatus = "not_detected"
		application.printer.info("未检测到支持的 Agent；可稍后执行 akey agent detect，或使用 akey run -- <command>")
		return nil
	}

	selected, proceed, err := selectDetectedAgent(application, candidates)
	if err != nil {
		return helperPartialError("Agent 选择无效", err)
	}
	if !proceed {
		view.AgentStatus = "skipped"
		application.printer.info("已跳过 Agent 基础初始化；Profile 与 Key 保持不变")
		return nil
	}
	view.Agent = selected.Name
	if selected.Name == "codex" {
		view.AgentStatus = "not_required"
		application.printer.info("Codex 无需独立全局配置，未修改任何文件；MCP 可通过 akey agent mcp 管理")
		return nil
	}

	application.printer.info(fmt.Sprintf("将为 %s 初始化基础配置：%s", selected.Name, selected.ConfigPath))
	application.printer.info("该操作只初始化 mcpServers，不会绑定模型或写入 API Key")
	confirmed, err := confirm(application.stdin, application.stderr, "确认修改上述 Agent 配置？")
	if err != nil {
		return err
	}
	if !confirmed {
		view.AgentStatus = "skipped"
		application.printer.info("已跳过 Agent 基础初始化；Profile 与 Key 保持不变")
		return nil
	}
	adapter, err := getAdapter(application, selected.Name)
	if err != nil {
		return helperPartialError("Agent 基础初始化失败", err)
	}
	changes, err := adapter.Configure(profile)
	if err != nil {
		return helperPartialError("Agent 基础初始化失败", err)
	}
	view.Configured = true
	if changes.Changed {
		view.AgentStatus = "configured"
		application.printer.success(fmt.Sprintf("Agent %s 基础配置已初始化", selected.Name))
	} else {
		view.AgentStatus = "unchanged"
		application.printer.info(fmt.Sprintf("Agent %s 现有配置无需变更", selected.Name))
	}
	return nil
}

func selectDetectedAgent(application *application, candidates []agent.InstallInfo) (agent.InstallInfo, bool, error) {
	if len(candidates) == 1 {
		return candidates[0], true, nil
	}
	_, _ = fmt.Fprintln(application.stderr, "检测到多个 Agent：")
	for index, info := range candidates {
		_, _ = fmt.Fprintf(application.stderr, "  %d) %s\n", index+1, info.Name)
	}
	choice, err := readLine(application.stdin, application.stderr, "请选择要初始化的 Agent [1]（0 跳过）: ")
	if err != nil {
		return agent.InstallInfo{}, false, err
	}
	if choice == "" {
		choice = "1"
	}
	index, err := strconv.Atoi(choice)
	if err != nil || index < 0 || index > len(candidates) {
		return agent.InstallInfo{}, false, apperr.NewUsage("invalid_agent_selection", "Agent 序号无效", fmt.Sprintf("请输入 0 到 %d", len(candidates)))
	}
	if index == 0 {
		return agent.InstallInfo{}, false, nil
	}
	return candidates[index-1], true, nil
}

func helperPartialError(step string, cause error) error {
	message := strings.TrimSpace(step)
	return apperr.NewRuntime("helper_partial", "基础配置已完成，但"+message, "已完成的 Profile、Key 和默认环境不会自动删除；请根据提示继续配置 Agent 或 MCP", cause)
}
