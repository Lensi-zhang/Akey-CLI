package main

import (
	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/envinject"

	"github.com/spf13/cobra"
)

func newEnvCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "env",
		Short: "生成手动 Shell 会话的环境脚本",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newEnvExportCommand(application), newEnvUnsetCommand(application))
	return command
}

func newEnvExportCommand(application *application) *cobra.Command {
	var shell string
	command := &cobra.Command{
		Use:   "export --shell <bash|zsh|fish|powershell|cmd>",
		Short: "输出供显式 eval 使用的环境脚本",
		Long:  "输出包含真实密钥的环境脚本。优先使用 akey run；eval 后密钥会存在于当前 Shell，使用完毕请执行 env unset 生成的清理脚本。",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if application.output == "json" {
				return apperr.NewUsage("env_json_forbidden", "env 命令不支持 JSON 输出", "请使用默认 table 输出并仅通过 eval 消费脚本")
			}
			if err := validateShell(shell); err != nil {
				return err
			}
			profile, err := application.selectedProfile()
			if err != nil {
				return err
			}
			secret, err := application.readSecret(profile)
			if err != nil {
				return err
			}
			script, err := envinject.Export(shell, profile, secret)
			if err != nil {
				return apperr.NewUsage("env_export_failed", "无法生成环境导出脚本", "请检查 Shell、环境前缀及变量值", err)
			}
			application.printer.data(script)
			return nil
		},
	}
	command.Flags().StringVar(&shell, "shell", "", "目标 Shell：bash、zsh、fish、powershell 或 cmd")
	return command
}

func newEnvUnsetCommand(application *application) *cobra.Command {
	var shell string
	command := &cobra.Command{
		Use:   "unset --shell <bash|zsh|fish|powershell|cmd>",
		Short: "输出清理当前 Shell 环境变量的脚本",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if application.output == "json" {
				return apperr.NewUsage("env_json_forbidden", "env 命令不支持 JSON 输出", "请使用默认 table 输出")
			}
			if err := validateShell(shell); err != nil {
				return err
			}
			profile, err := application.selectedProfile()
			if err != nil {
				return err
			}
			script, err := envinject.Unset(shell, profile.EnvPrefix)
			if err != nil {
				return apperr.NewUsage("env_unset_failed", "无法生成环境清理脚本", "请检查 Shell 和环境前缀", err)
			}
			application.printer.data(script)
			return nil
		},
	}
	command.Flags().StringVar(&shell, "shell", "", "目标 Shell：bash、zsh、fish、powershell 或 cmd")
	return command
}

func validateShell(shell string) error {
	switch shell {
	case "bash", "zsh", "fish", "powershell", "cmd":
		return nil
	default:
		return apperr.NewUsage("invalid_shell", "必须通过 --shell 指定 bash、zsh、fish、powershell 或 cmd", "请提供受支持的 Shell")
	}
}
