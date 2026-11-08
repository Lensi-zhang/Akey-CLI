package main

import (
	"errors"
	"fmt"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/format"

	"github.com/spf13/cobra"
)

type authStatusView struct {
	Profile string `json:"profile"`
	Backend string `json:"backend,omitempty"`
	Status  string `json:"status"`
}

type authLogoutView struct {
	Profile   string `json:"profile"`
	LoggedOut bool   `json:"logged_out"`
}

func newAuthCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "auth",
		Short: "管理 Profile 对应的系统密钥环凭证",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newAuthLoginCommand(application),
		newAuthLogoutCommand(application),
		newAuthStatusCommand(application),
	)
	return command
}

func newAuthLoginCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "通过隐藏输入设置或轮换 API Key",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := application.selectedProfile()
			if err != nil {
				return err
			}
			if err := application.requireInteractive(); err != nil {
				return err
			}
			if err := application.promptAndSetSecret(profile); err != nil {
				return err
			}
			application.printer.success(fmt.Sprintf("Profile %s 的 Key 已安全保存", profile.Name))
			view := application.authStatus(profile)
			return application.emit(view, authStatusTable(view))
		},
	}
}

func newAuthLogoutCommand(application *application) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "logout",
		Short: "删除当前 Profile 的 Key，保留元数据",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := application.selectedProfile()
			if err != nil {
				return err
			}
			if !yes {
				if application.nonInteractive {
					return apperr.NewUsage("confirmation_required", "非交互模式删除 Key 必须指定 --yes", "请确认影响后添加 --yes")
				}
				confirmed, err := confirm(application.stdin, application.stderr, fmt.Sprintf("确认删除 Profile %s 的 Key？", profile.Name))
				if err != nil {
					return err
				}
				if !confirmed {
					return apperr.NewCanceled("logout_canceled", "退出操作已取消", "未删除 Key")
				}
			}
			store, err := application.secretStore()
			if err != nil {
				return err
			}
			if err := store.Delete(profile); err != nil {
				return err
			}
			application.printer.success(fmt.Sprintf("Profile %s 的 Key 已删除", profile.Name))
			view := authLogoutView{Profile: profile.Name, LoggedOut: true}
			return application.emit(view, format.Table([]string{"PROFILE", "LOGGED_OUT"}, [][]string{{view.Profile, "true"}}))
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "确认删除，不进入交互")
	return command
}

func newAuthStatusCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看 Key 配置状态，不读取或显示 Key 内容",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := application.selectedProfile()
			if err != nil {
				return err
			}
			view := application.authStatus(profile)
			return application.emit(view, authStatusTable(view))
		},
	}
}

func (application *application) promptAndSetSecret(profile config.Profile) error {
	secret, err := promptSecret(application.stdin, application.stderr, fmt.Sprintf("输入 Profile %s 的 API Key: ", profile.Name))
	if err != nil {
		switch {
		case errors.Is(err, errInputCanceled):
			return apperr.NewCanceled("key_input_canceled", "密钥录入已取消", "可稍后通过 auth login 重新录入")
		case errors.Is(err, errEmptySecret):
			return apperr.NewUsage("empty_key", "API Key 不能为空", "请重新执行命令并输入非空密钥")
		default:
			return apperr.NewRuntime("key_input_failed", "无法读取隐藏密钥输入", "请确认终端支持交互输入后重试", err)
		}
	}
	store, err := application.secretStore()
	if err != nil {
		return err
	}
	return store.Set(profile, secret)
}

func (application *application) authStatus(profile config.Profile) authStatusView {
	view := authStatusView{Profile: profile.Name, Status: keyUnavailable}
	store, err := application.secretStore()
	if err != nil {
		return view
	}
	status, err := store.Status(profile)
	view.Backend = status.Backend
	if err != nil {
		return view
	}
	view.Status = keyUnconfigured
	if status.Configured {
		view.Status = keyConfigured
	}
	return view
}

func authStatusTable(view authStatusView) string {
	return format.Table([]string{"PROFILE", "BACKEND", "STATUS"}, [][]string{{view.Profile, view.Backend, view.Status}})
}
