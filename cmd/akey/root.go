package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	childprocess "github.com/Lensi-zhang/Akey-CLI/internal/process"

	"github.com/spf13/cobra"
)

func execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	application, err := newApplication(stdin, stdout, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "✗ 初始化失败: %v\n", err)
		return 1
	}

	root := newRootCommand(application)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		err = normalizeExecuteError(err)
		application.renderError(err)
		return errorExitCode(err)
	}
	return 0
}

// normalizeExecuteError 防止未分类的底层错误文本直接输出；子进程退出错误必须原样透传。
func normalizeExecuteError(err error) error {
	var exitError *childprocess.ExitError
	if errors.As(err, &exitError) {
		return err
	}
	var applicationError *apperr.Error
	if errors.As(err, &applicationError) {
		return err
	}
	if strings.HasPrefix(err.Error(), "unknown command") || strings.HasPrefix(err.Error(), "unknown help topic") {
		return apperr.NewUsage("unknown_command", "未知命令", "请使用 --help 查看支持的命令", err)
	}
	return apperr.NewRuntime("command_failed", "命令执行失败", "请检查输入和运行环境后重试", err)
}

func newRootCommand(application *application) *cobra.Command {
	root := &cobra.Command{
		Use:           "akey",
		Short:         "安全 API Key 与本地 AI Agent 管理工具",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(command *cobra.Command, args []string) error {
			if application.output != "table" && application.output != "json" {
				return apperr.NewUsage("invalid_output", "输出格式仅支持 table 或 json", "请通过 --output 指定有效格式")
			}
			application.refreshPrinter()
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			showVersion, _ := command.Flags().GetBool("version")
			if showVersion {
				table := fmt.Sprintf("akey %s\ncommit: %s\nbuilt: %s\n", version, commit, buildDate)
				return application.emit(map[string]string{
					"version": version,
					"commit":  commit,
					"built":   buildDate,
				}, table)
			}
			return runDefaultHelper(application)
		},
	}
	root.SetIn(application.stdin)
	root.SetOut(application.stdout)
	root.SetErr(application.stderr)
	root.SetFlagErrorFunc(func(command *cobra.Command, err error) error {
		return apperr.NewUsage("invalid_flags", "命令参数无效", "请使用 --help 查看用法", err)
	})
	root.PersistentFlags().StringVarP(&application.profileName, "profile", "p", "", "指定 Profile 名称")
	root.PersistentFlags().StringVarP(&application.output, "output", "o", "table", "输出格式：table 或 json")
	root.PersistentFlags().BoolVar(&application.noColor, "no-color", false, "禁用颜色及终端样式")
	root.PersistentFlags().BoolVar(&application.nonInteractive, "non-interactive", false, "禁止交互询问")
	root.PersistentFlags().BoolVar(&application.verbose, "verbose", false, "输出安全诊断详情")
	root.Flags().BoolP("version", "v", false, "查看版本")
	root.AddCommand(
		newAuthCommand(application),
		newProfileCommand(application),
		newRunCommand(application),
		newEnvCommand(application),
		newAgentCommand(application),
		newHelperCommand(application),
	)
	wrapPositionalErrors(root)
	return root
}

func wrapPositionalErrors(command *cobra.Command) {
	if command.Args != nil {
		validate := command.Args
		command.Args = func(command *cobra.Command, args []string) error {
			if err := validate(command, args); err != nil {
				return apperr.NewUsage("invalid_arguments", "命令参数数量或格式不正确", "请使用 --help 查看用法", err)
			}
			return nil
		}
	}
	for _, child := range command.Commands() {
		wrapPositionalErrors(child)
	}
}
