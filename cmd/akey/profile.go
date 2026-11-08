package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/format"
	keyringstore "github.com/Lensi-zhang/Akey-CLI/internal/keyring"
	"github.com/Lensi-zhang/Akey-CLI/internal/validate"

	"github.com/spf13/cobra"
)

const (
	keyConfigured   = "configured"
	keyUnconfigured = "unconfigured"
	keyUnavailable  = "unavailable"
)

type keyView struct {
	Status  string `json:"status"`
	Backend string `json:"backend,omitempty"`
}

type profileView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	BaseURL   string    `json:"base_url"`
	Model     string    `json:"model"`
	EnvPrefix string    `json:"env_prefix"`
	Note      string    `json:"note"`
	Current   bool      `json:"current"`
	Key       keyView   `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type profileDeleteView struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

func newProfileCommand(application *application) *cobra.Command {
	command := &cobra.Command{
		Use:   "profile",
		Short: "管理隔离的 API 环境",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newProfileCreateCommand(application),
		newProfileListCommand(application),
		newProfileShowCommand(application),
		newProfileUpdateCommand(application),
		newProfileRenameCommand(application),
		newProfileUseCommand(application),
		newProfileCurrentCommand(application),
		newProfileDeleteCommand(application),
	)
	return command
}

func newProfileCreateCommand(application *application) *cobra.Command {
	var provider, baseURL, model, envPrefix, note string
	command := &cobra.Command{
		Use:   "create <name>",
		Short: "创建 Profile 并引导隐藏录入 Key",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := validate.ProfileName(args[0]); err != nil {
				return err
			}
			var err error
			if provider == "" {
				if err = application.requireInteractive(); err != nil {
					return err
				}
				provider, err = promptLine(application.stdin, application.stderr, "Provider: ")
				if err != nil {
					return err
				}
			}
			if baseURL == "" {
				if err = application.requireInteractive(); err != nil {
					return err
				}
				baseURL, err = promptLine(application.stdin, application.stderr, "Base URL: ")
				if err != nil {
					return err
				}
			}
			if model == "" {
				if err = application.requireInteractive(); err != nil {
					return err
				}
				model, err = promptLine(application.stdin, application.stderr, "Model 或 Endpoint ID: ")
				if err != nil {
					return err
				}
			}

			created, err := application.config.Create(config.Profile{
				Name:      args[0],
				Provider:  provider,
				BaseURL:   baseURL,
				Model:     model,
				EnvPrefix: envPrefix,
				Note:      note,
			})
			if err != nil {
				return err
			}
			if !application.nonInteractive {
				if err := application.promptAndSetSecret(created); err != nil {
					return profileCreatedWithoutKey(created.Name, err)
				}
				application.printer.success(fmt.Sprintf("Profile %s 已创建，Key 已安全保存", created.Name))
			} else {
				application.printer.info(fmt.Sprintf("Profile %s 已创建；请通过 auth login 隐藏录入 Key", created.Name))
			}
			view, err := application.makeProfileView(created)
			if err != nil {
				return err
			}
			return application.emit(view, profileTable(view))
		},
	}
	command.Flags().StringVar(&provider, "provider", "", "API 服务提供方")
	command.Flags().StringVar(&baseURL, "base-url", "", "API Base URL")
	command.Flags().StringVar(&model, "model", "", "模型或 Endpoint ID")
	command.Flags().StringVar(&envPrefix, "env-prefix", config.DefaultEnvPrefix, "环境变量前缀")
	command.Flags().StringVar(&note, "note", "", "备注")
	return command
}

func newProfileListCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出全部 Profile",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			state, err := application.config.Load()
			if err != nil {
				return err
			}
			views := make([]profileView, 0, len(state.Profiles))
			rows := make([][]string, 0, len(state.Profiles))
			for _, profile := range state.Profiles {
				view := application.profileView(profile, state.CurrentProfile)
				views = append(views, view)
				current := ""
				if view.Current {
					current = "*"
				}
				rows = append(rows, []string{current, view.Name, view.Provider, view.Model, view.BaseURL, view.Key.Status, view.Note})
			}
			if len(views) == 0 {
				application.printer.info("尚未创建 Profile")
			}
			return application.emit(views, format.Table([]string{"CURRENT", "NAME", "PROVIDER", "MODEL", "BASE_URL", "KEY", "NOTE"}, rows))
		},
	}
}

func newProfileShowCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "查看 Profile 元数据与 Key 状态",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var profile config.Profile
			var err error
			if len(args) == 1 {
				profile, err = application.config.Get(args[0])
			} else {
				profile, err = application.selectedProfile()
			}
			if err != nil {
				return err
			}
			view, err := application.makeProfileView(profile)
			if err != nil {
				return err
			}
			return application.emit(view, profileTable(view))
		},
	}
}

func newProfileUpdateCommand(application *application) *cobra.Command {
	var provider, baseURL, model, envPrefix, note string
	command := &cobra.Command{
		Use:   "update <name>",
		Short: "更新显式指定的 Profile 元数据",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			flags := command.Flags()
			if !flags.Changed("provider") && !flags.Changed("base-url") && !flags.Changed("model") && !flags.Changed("env-prefix") && !flags.Changed("note") {
				return apperr.NewUsage("empty_update", "没有需要更新的字段", "请至少提供一个元数据选项")
			}
			patch := config.Patch{}
			if flags.Changed("provider") {
				patch.Provider = &provider
			}
			if flags.Changed("base-url") {
				patch.BaseURL = &baseURL
			}
			if flags.Changed("model") {
				patch.Model = &model
			}
			if flags.Changed("env-prefix") {
				patch.EnvPrefix = &envPrefix
			}
			if flags.Changed("note") {
				patch.Note = &note
			}
			updated, err := application.config.Update(args[0], patch)
			if err != nil {
				return err
			}
			application.printer.success(fmt.Sprintf("Profile %s 已更新", updated.Name))
			view, err := application.makeProfileView(updated)
			if err != nil {
				return err
			}
			return application.emit(view, profileTable(view))
		},
	}
	command.Flags().StringVar(&provider, "provider", "", "API 服务提供方")
	command.Flags().StringVar(&baseURL, "base-url", "", "API Base URL")
	command.Flags().StringVar(&model, "model", "", "模型或 Endpoint ID")
	command.Flags().StringVar(&envPrefix, "env-prefix", "", "环境变量前缀")
	command.Flags().StringVar(&note, "note", "", "备注；传空字符串可清除")
	return command
}

func newProfileRenameCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old-name> <new-name>",
		Short: "重命名 Profile 并同步 Agent 引用",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			renamed, err := application.config.Rename(args[0], args[1])
			if err != nil {
				return err
			}
			changes, syncErr := application.agents.RenameProfile(args[0], renamed)
			if syncErr != nil {
				completed := 0
				for _, change := range changes {
					if change.Changed {
						completed++
					}
				}
				hint := "请检查 Agent MCP 配置中的 --profile 参数后重试同步"
				if completed > 0 {
					hint = fmt.Sprintf("已完成 %d 项 Agent 配置变更，未执行跨 Agent 回滚；%s", completed, hint)
				}
				return apperr.NewRuntime(
					"agent_profile_rename_failed",
					fmt.Sprintf("Profile 已重命名为 %q，但 Agent 引用同步失败", renamed.Name),
					hint,
					syncErr,
				)
			}
			application.printer.success(fmt.Sprintf("Profile 已重命名为 %s", renamed.Name))
			view, err := application.makeProfileView(renamed)
			if err != nil {
				return err
			}
			return application.emit(view, profileTable(view))
		},
	}
}

func newProfileUseCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "切换默认 Profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := application.config.Use(args[0]); err != nil {
				return err
			}
			profile, err := application.config.Get(args[0])
			if err != nil {
				return err
			}
			application.printer.success(fmt.Sprintf("默认 Profile 已切换为 %s", profile.Name))
			view, err := application.makeProfileView(profile)
			if err != nil {
				return err
			}
			return application.emit(view, profileTable(view))
		},
	}
}

func newProfileCurrentCommand(application *application) *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "显示默认 Profile",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := application.config.Current()
			if err != nil {
				return err
			}
			view, err := application.makeProfileView(profile)
			if err != nil {
				return err
			}
			return application.emit(view, profileTable(view))
		},
	}
}

func newProfileDeleteCommand(application *application) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "delete <name>",
		Short: "删除 Profile 元数据及其 Key",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := application.config.Get(args[0])
			if err != nil {
				return err
			}
			if !yes {
				if application.nonInteractive {
					return apperr.NewUsage("confirmation_required", "非交互模式删除 Profile 必须指定 --yes", "请确认影响后添加 --yes")
				}
				confirmed, err := confirm(application.stdin, application.stderr, fmt.Sprintf("确认删除 Profile %s 及其 Key？", profile.Name))
				if err != nil {
					return err
				}
				if !confirmed {
					return apperr.NewCanceled("delete_canceled", "删除操作已取消", "未修改 Profile 或 Key")
				}
			}

			store, err := application.secretStore()
			if err != nil {
				return err
			}
			secret, err := store.Get(profile)
			hadSecret := err == nil
			if err != nil && !errors.Is(err, keyringstore.ErrNotConfigured) {
				return err
			}
			if err := store.Delete(profile); err != nil {
				return err
			}
			if err := application.config.Delete(profile.Name); err != nil {
				if hadSecret {
					if restoreErr := store.Set(profile, secret); restoreErr != nil {
						return apperr.NewConfig("profile_delete_rollback_failed", "Profile 元数据删除失败，且 Key 恢复失败", "请立即检查系统密钥环和配置文件", errors.Join(err, restoreErr))
					}
					return apperr.NewConfig("profile_delete_rolled_back", "Profile 元数据删除失败，Key 已恢复", "请检查配置文件后重试", err)
				}
				return err
			}
			application.printer.success(fmt.Sprintf("Profile %s 及其 Key 已删除", profile.Name))
			view := profileDeleteView{Name: profile.Name, Deleted: true}
			return application.emit(view, format.Table([]string{"NAME", "DELETED"}, [][]string{{view.Name, "true"}}))
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "确认删除，不进入交互")
	return command
}

func profileTable(view profileView) string {
	return format.Table([]string{"FIELD", "VALUE"}, [][]string{
		{"ID", view.ID},
		{"NAME", view.Name},
		{"PROVIDER", view.Provider},
		{"BASE_URL", view.BaseURL},
		{"MODEL", view.Model},
		{"ENV_PREFIX", view.EnvPrefix},
		{"NOTE", view.Note},
		{"CURRENT", yesNo(view.Current)},
		{"KEY", view.Key.Status},
		{"KEYRING_BACKEND", view.Key.Backend},
		{"CREATED_AT", view.CreatedAt.Format(time.RFC3339)},
		{"UPDATED_AT", view.UpdatedAt.Format(time.RFC3339)},
	})
}

func (application *application) makeProfileView(profile config.Profile) (profileView, error) {
	state, err := application.config.Load()
	if err != nil {
		return profileView{}, err
	}
	return application.profileView(profile, state.CurrentProfile), nil
}

func (application *application) profileView(profile config.Profile, currentName string) profileView {
	return profileView{
		ID:        profile.ID,
		Name:      profile.Name,
		Provider:  profile.Provider,
		BaseURL:   profile.BaseURL,
		Model:     profile.Model,
		EnvPrefix: profile.EnvPrefix,
		Note:      profile.Note,
		Current:   profile.Name == currentName,
		Key:       application.keyView(profile),
		CreatedAt: profile.CreatedAt,
		UpdatedAt: profile.UpdatedAt,
	}
}

func (application *application) keyView(profile config.Profile) keyView {
	store, err := application.secretStore()
	if err != nil {
		return keyView{Status: keyUnavailable}
	}
	status, err := store.Status(profile)
	view := keyView{Status: keyUnavailable, Backend: status.Backend}
	if err != nil {
		return view
	}
	if status.Configured {
		view.Status = keyConfigured
	} else {
		view.Status = keyUnconfigured
	}
	return view
}

func profileCreatedWithoutKey(name string, cause error) error {
	kind := apperr.KindRuntime
	var applicationError *apperr.Error
	if errors.As(cause, &applicationError) {
		kind = applicationError.Kind
	}
	return apperr.New(kind, "profile_created_without_key", "Profile 已创建，但 Key 未配置", fmt.Sprintf("请执行 akey auth login --profile %s 完成密钥录入", name), cause)
}
