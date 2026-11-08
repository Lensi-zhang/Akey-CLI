package main

import (
	"errors"
	"os"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/envinject"
	childprocess "github.com/Lensi-zhang/Akey-CLI/internal/process"

	"github.com/spf13/cobra"
)

func newRunCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "run -- <command> [args...]",
		Short: "在纯净子进程环境中安全运行命令",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := application.selectedProfile()
			if err != nil {
				return err
			}
			secret, err := application.readSecret(profile)
			if err != nil {
				return err
			}
			environment := envinject.Build(os.Environ(), profile, secret)
			_, runErr := childprocess.Run(args[0], args[1:], environment, application.stdin, application.stdout, application.stderr)
			if runErr == nil {
				return nil
			}
			var startError *childprocess.StartError
			if errors.As(runErr, &startError) {
				return apperr.NewRuntime("process_start_failed", "无法启动子进程", "请检查命令是否存在以及执行权限", runErr)
			}
			var waitError *childprocess.WaitError
			if errors.As(runErr, &waitError) {
				return apperr.NewRuntime("process_wait_failed", "无法取得子进程退出状态", "请检查子进程状态", runErr)
			}
			return runErr
		},
	}
}
