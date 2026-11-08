package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/format"
	"github.com/Lensi-zhang/Akey-CLI/internal/mcp"
	"github.com/Lensi-zhang/Akey-CLI/internal/validate"

	"github.com/spf13/cobra"
)

type helperView struct {
	Profile    string `json:"profile"`
	Current    bool   `json:"current"`
	Agent      string `json:"agent,omitempty"`
	Configured bool   `json:"configured"`
	MCPAction  string `json:"mcp_action,omitempty"`
	MCPServer  string `json:"mcp_server,omitempty"`
}

func newHelperCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "helper",
		Short: "一站式配置 Profile、Key、Agent 与 MCP",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if err := application.requireInteractive(); err != nil {
				return err
			}
			return runHelper(application)
		},
	}
}

func runHelper(application *application) error {
	name, err := promptLine(application.stdin, application.stderr, "Profile 名称: ")
	if err != nil {
		return err
	}
	if err := validate.ProfileName(name); err != nil {
		return err
	}
	provider, err := promptLine(application.stdin, application.stderr, "Provider: ")
	if err != nil {
		return err
	}
	baseURL, err := promptLine(application.stdin, application.stderr, "Base URL: ")
	if err != nil {
		return err
	}
	model, err := promptLine(application.stdin, application.stderr, "Model 或 Endpoint ID: ")
	if err != nil {
		return err
	}
	envPrefix, err := readLine(application.stdin, application.stderr, "环境变量前缀 [API]: ")
	if err != nil {
		return err
	}
	if envPrefix == "" {
		envPrefix = config.DefaultEnvPrefix
	}
	note, err := readLine(application.stdin, application.stderr, "备注（可选）: ")
	if err != nil {
		return err
	}
	secret, err := promptSecret(application.stdin, application.stderr, "API Key: ")
	if err != nil {
		if errors.Is(err, errInputCanceled) {
			return apperr.NewCanceled("helper_canceled", "向导已取消", "未创建 Profile、Key 或 Agent 配置")
		}
		if errors.Is(err, errEmptySecret) {
			return apperr.NewUsage("empty_key", "API Key 不能为空", "未创建任何配置，请重新运行 helper")
		}
		return apperr.NewRuntime("key_input_failed", "无法读取隐藏密钥输入", "未创建任何配置，请检查终端后重试", err)
	}
	created, err := application.config.Create(config.Profile{
		Name:      name,
		Provider:  provider,
		BaseURL:   baseURL,
		Model:     model,
		EnvPrefix: envPrefix,
		Note:      note,
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
	application.printer.success(fmt.Sprintf("Profile %s 与 Key 已创建，并设为默认环境", created.Name))

	view := helperView{Profile: created.Name, Current: true}
	configureAgent, err := confirm(application.stdin, application.stderr, "是否现在配置 Agent？")
	if err != nil {
		return err
	}
	if configureAgent {
		agentName, err := promptLine(application.stdin, application.stderr, "Agent (trae/claude-code/codex): ")
		if err != nil {
			return err
		}
		adapter, err := getAdapter(application, agentName)
		if err != nil {
			return helperPartialError("Agent 配置失败", err)
		}
		if _, err := adapter.Configure(created); err != nil {
			return helperPartialError("Agent 配置失败", err)
		}
		view.Agent = agentName
		view.Configured = true
		application.printer.success(fmt.Sprintf("Agent %s 配置完成", agentName))
	}

	manageMCP, err := confirm(application.stdin, application.stderr, "是否挂载或移除 MCP？")
	if err != nil {
		return err
	}
	if manageMCP {
		if err := runHelperMCP(application, created, &view); err != nil {
			return err
		}
	}
	return application.emit(view, format.Table([]string{"PROFILE", "CURRENT", "AGENT", "CONFIGURED", "MCP_ACTION", "MCP_SERVER"}, [][]string{{
		view.Profile,
		yesNo(view.Current),
		view.Agent,
		yesNo(view.Configured),
		view.MCPAction,
		view.MCPServer,
	}}))
}

func runHelperMCP(application *application, profile config.Profile, view *helperView) error {
	action, err := promptLine(application.stdin, application.stderr, "MCP 操作 (add/remove): ")
	if err != nil {
		return err
	}
	if action != "add" && action != "remove" {
		return apperr.NewUsage("invalid_mcp_action", "MCP 操作仅支持 add 或 remove", "Profile 与 Key 已创建，可稍后通过 agent mcp 管理")
	}
	agentName, err := readLine(application.stdin, application.stderr, fmt.Sprintf("Agent [%s]: ", view.Agent))
	if err != nil {
		return err
	}
	if agentName == "" {
		agentName = view.Agent
	}
	if agentName == "" {
		agentName, err = promptLine(application.stdin, application.stderr, "Agent (trae/claude-code/codex): ")
		if err != nil {
			return err
		}
	}
	serverName, err := promptLine(application.stdin, application.stderr, "MCP Server 名称: ")
	if err != nil {
		return err
	}
	adapter, err := getAdapter(application, agentName)
	if err != nil {
		return helperPartialError("MCP 操作失败", err)
	}

	if action == "add" {
		executable, err := readLine(application.stdin, application.stderr, "自定义启动命令（留空使用内置模板）: ")
		if err != nil {
			return err
		}
		var server mcp.Server
		if executable == "" {
			server, err = mcp.Builtin(serverName, profile.Name)
		} else {
			server, err = mcp.Custom(serverName, executable, nil, profile.Name)
		}
		if err != nil {
			return helperPartialError("MCP 挂载参数无效", err)
		}
		if _, err := exec.LookPath(server.Command); err != nil {
			return helperPartialError("MCP 启动命令不可用", apperr.NewNotFound("mcp_command_not_found", "找不到 MCP 启动命令", "请先安装服务或提供有效命令"))
		}
		if _, err := adapter.AddMCP(profile, server); err != nil {
			return helperPartialError("MCP 挂载失败", err)
		}
	} else {
		confirmed, err := confirm(application.stdin, application.stderr, fmt.Sprintf("确认从 %s 移除 %s？", agentName, serverName))
		if err != nil {
			return err
		}
		if !confirmed {
			application.printer.info("已跳过 MCP 移除；此前 Profile、Key 和 Agent 配置保持不变")
			return nil
		}
		if _, err := adapter.RemoveMCP(serverName); err != nil {
			return helperPartialError("MCP 移除失败", err)
		}
	}
	view.MCPAction = action
	view.MCPServer = serverName
	application.printer.success(fmt.Sprintf("MCP %s 操作完成: %s", action, serverName))
	return nil
}

func helperPartialError(step string, cause error) error {
	message := strings.TrimSpace(step)
	return apperr.NewRuntime("helper_partial", "基础配置已完成，但"+message, "已完成的 Profile、Key 和默认环境不会自动删除；请根据提示继续配置 Agent 或 MCP", cause)
}
