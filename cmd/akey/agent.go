package main

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/agent"
	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/format"
	"github.com/Lensi-zhang/Akey-CLI/internal/mcp"

	"github.com/spf13/cobra"
)

func newAgentCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "agent",
		Short: "管理本地 AI Agent 与 MCP 配置",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newAgentListCommand(application),
		newAgentDetectCommand(application),
		newAgentStatusCommand(application),
		newAgentConfigureCommand(application),
		newAgentMCPCommand(application),
	)
	return command
}

func newAgentListCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出已注册的 Agent 适配器",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			adapters := application.agents.List()
			items := make([]map[string]string, 0, len(adapters))
			rows := make([][]string, 0, len(adapters))
			for _, adapter := range adapters {
				items = append(items, map[string]string{"name": adapter.Name()})
				rows = append(rows, []string{adapter.Name()})
			}
			if len(rows) == 0 {
				application.printer.info("没有已注册的 Agent")
			}
			return application.emit(items, format.Table([]string{"AGENT"}, rows))
		},
	}
}

func newAgentDetectCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "检测本机 Agent 安装与配置路径",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			items, err := detectAgents(application)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(items))
			for _, info := range items {
				rows = append(rows, []string{info.Name, yesNo(info.Installed), info.ExecutablePath, info.ConfigPath, yesNo(info.ConfigExists)})
			}
			return application.emit(items, format.Table([]string{"AGENT", "INSTALLED", "EXECUTABLE", "CONFIG", "CONFIG_EXISTS"}, rows))
		},
	}
}

func detectAgents(application *application) ([]agent.InstallInfo, error) {
	adapters := application.agents.List()
	items := make([]agent.InstallInfo, 0, len(adapters))
	for _, adapter := range adapters {
		info, err := adapter.Detect()
		if err != nil {
			return nil, normalizeAgentError(err, "agent_detect_failed", fmt.Sprintf("检测 Agent %s 失败", adapter.Name()), "请检查 Agent 安装和配置路径")
		}
		items = append(items, info)
	}
	return items, nil
}

func newAgentStatusCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "status [agent]",
		Short: "查看 Agent 配置状态",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			adapters, err := selectAdapters(application.agents, args)
			if err != nil {
				return err
			}
			items := make([]agent.Status, 0, len(adapters))
			rows := make([][]string, 0, len(adapters))
			for _, adapter := range adapters {
				status, err := adapter.Status()
				if err != nil {
					return normalizeAgentError(err, "agent_status_failed", fmt.Sprintf("读取 Agent %s 状态失败", adapter.Name()), "请检查 Agent 配置文件是否有效")
				}
				items = append(items, status)
				rows = append(rows, []string{status.Name, yesNo(status.Installed), yesNo(status.Configured), strings.Join(status.Profiles, ","), fmt.Sprintf("%d", status.MCPCount), status.ConfigPath})
			}
			return application.emit(items, format.Table([]string{"AGENT", "INSTALLED", "CONFIGURED", "PROFILES", "MCP", "CONFIG"}, rows))
		},
	}
}

func newAgentConfigureCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "configure <agent>",
		Short: "配置 Agent 使用 akey 运行时注入",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := application.selectedProfile()
			if err != nil {
				return err
			}
			adapter, err := getAdapter(application, args[0])
			if err != nil {
				return err
			}
			change, err := adapter.Configure(profile)
			if err != nil {
				return normalizeAgentError(err, "agent_configure_failed", "Agent 配置失败", "请检查配置文件权限和 Agent 状态")
			}
			application.printer.success(fmt.Sprintf("Agent %s 配置完成", args[0]))
			return application.emit(change, fmt.Sprintf("AGENT\t%s\nCONFIG\t%s\n", change.Agent, change.ConfigPath))
		},
	}
}

func newAgentMCPCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "mcp",
		Short: "管理 Agent 的 MCP Server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newAgentMCPListCommand(application),
		newAgentMCPAddCommand(application),
		newAgentMCPRemoveCommand(application),
	)
	return command
}

func newAgentMCPListCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "list <agent>",
		Short: "列出 Agent 的 MCP Server",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			adapter, err := getAdapter(application, args[0])
			if err != nil {
				return err
			}
			document, err := adapter.ListMCP()
			if err != nil {
				return normalizeAgentError(err, "agent_mcp_list_failed", "无法读取 MCP 配置", "请检查 Agent 配置和 CLI 输出")
			}
			rows := make([][]string, 0, len(document.MCPServers))
			names := make([]string, 0, len(document.MCPServers))
			for name := range document.MCPServers {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				entry := document.MCPServers[name]
				profile := "-"
				if value, managed := mcp.ManagedProfile(entry); managed {
					profile = value
				}
				rows = append(rows, []string{name, profile, entry.Command, strings.Join(entry.Args, " ")})
			}
			if len(rows) == 0 {
				application.printer.info("没有 MCP Server")
			}
			return application.emit(document, format.Table([]string{"SERVER", "PROFILE", "COMMAND", "ARGS"}, rows))
		},
	}
}

func newAgentMCPAddCommand(application *application) *cobra.Command {
	var executable string
	var serverArgs []string
	command := &cobra.Command{
		Use:   "add <agent> <server>",
		Short: "挂载内置或自定义 MCP Server",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if executable == "" && len(serverArgs) > 0 {
				return apperr.NewUsage("mcp_args_without_command", "--arg 只能与 --command 一起使用", "请提供自定义 MCP 启动命令")
			}
			profile, err := application.selectedProfile()
			if err != nil {
				return err
			}
			var server mcp.Server
			if executable == "" {
				server, err = mcp.Builtin(args[1], profile.Name)
			} else {
				server, err = mcp.Custom(args[1], executable, serverArgs, profile.Name)
			}
			if err != nil {
				return apperr.NewUsage("invalid_mcp_server", "MCP 服务参数无效", "请检查服务名称、启动命令和参数", err)
			}
			if _, err := exec.LookPath(server.Command); err != nil {
				return apperr.NewNotFound("mcp_command_not_found", fmt.Sprintf("MCP 启动命令 %q 不可用", server.Command), "请先安装服务或通过 --command 指定有效命令")
			}
			adapter, err := getAdapter(application, args[0])
			if err != nil {
				return err
			}
			change, err := adapter.AddMCP(profile, server)
			if err != nil {
				return normalizeAgentError(err, "agent_mcp_add_failed", "MCP 挂载失败", "请检查服务名称、Profile 和 Agent 配置")
			}
			application.printer.success(fmt.Sprintf("MCP %s 已挂载到 %s", args[1], args[0]))
			return application.emit(change, fmt.Sprintf("SERVER\t%s\nAGENT\t%s\nCONFIG\t%s\n", args[1], args[0], change.ConfigPath))
		},
	}
	command.Flags().StringVar(&executable, "command", "", "自定义 MCP 启动命令")
	command.Flags().StringArrayVar(&serverArgs, "arg", nil, "自定义 MCP 启动参数，可重复使用")
	return command
}

func newAgentMCPRemoveCommand(application *application) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "remove <agent> <server>",
		Short: "移除 akey 管理的 MCP Server",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				if application.nonInteractive {
					return apperr.NewUsage("confirmation_required", "非交互模式删除 MCP 必须显式指定 --yes", "请确认影响后添加 --yes")
				}
				confirmed, err := confirm(application.stdin, application.stderr, fmt.Sprintf("确认从 %s 移除 %s？", args[0], args[1]))
				if err != nil {
					return err
				}
				if !confirmed {
					return apperr.NewCanceled("mcp_remove_canceled", "MCP 移除操作已取消", "未修改 Agent 配置")
				}
			}
			adapter, err := getAdapter(application, args[0])
			if err != nil {
				return err
			}
			change, err := adapter.RemoveMCP(args[1])
			if err != nil {
				return normalizeAgentError(err, "agent_mcp_remove_failed", "MCP 移除失败", "请检查服务是否存在且由 akey 管理")
			}
			application.printer.success(fmt.Sprintf("MCP %s 已移除", args[1]))
			return application.emit(change, fmt.Sprintf("SERVER\t%s\nAGENT\t%s\nREMOVED\ttrue\n", args[1], args[0]))
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "确认移除，不进入交互")
	return command
}

func selectAdapters(registry *agent.Registry, names []string) ([]agent.Adapter, error) {
	if len(names) == 0 {
		return registry.List(), nil
	}
	adapter, err := registry.Get(names[0])
	if err != nil {
		if errors.Is(err, agent.ErrAdapterNotFound) {
			return nil, apperr.NewNotFound("agent_not_found", "Agent 适配器不存在", "请执行 akey agent list 查看支持的适配器", err)
		}
		return nil, apperr.NewRuntime("agent_registry_failed", "无法读取 Agent 适配器", "请重试", err)
	}
	return []agent.Adapter{adapter}, nil
}

func getAdapter(application *application, name string) (agent.Adapter, error) {
	adapter, err := application.agents.Get(name)
	if err == nil {
		return adapter, nil
	}
	if errors.Is(err, agent.ErrAdapterNotFound) {
		return nil, apperr.NewNotFound("agent_not_found", "Agent 适配器不存在", "请执行 akey agent list 查看支持的适配器", err)
	}
	return nil, apperr.NewRuntime("agent_registry_failed", "无法读取 Agent 适配器", "请重试", err)
}

// normalizeAgentError 将第三方输出限制在安全错误链内，渲染层不会展示底层文本。
func normalizeAgentError(err error, code, message, hint string) error {
	if err == nil {
		return nil
	}
	var applicationError *apperr.Error
	if errors.As(err, &applicationError) {
		return err
	}
	switch {
	case errors.Is(err, agent.ErrNotFound), errors.Is(err, agent.ErrCodexUnavailable):
		return apperr.NewNotFound(code, message, hint, err)
	case errors.Is(err, agent.ErrConflict):
		return apperr.NewConflict(code, message, hint, err)
	case errors.Is(err, agent.ErrInvalidConfig):
		return apperr.NewConfig(code, message, hint, err)
	default:
		return apperr.NewRuntime(code, message, hint, err)
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
