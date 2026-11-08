package main

import (
	"fmt"
	"io"
	"sync"

	"github.com/Lensi-zhang/Akey-CLI/internal/agent"
	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
	"github.com/Lensi-zhang/Akey-CLI/internal/config"
	"github.com/Lensi-zhang/Akey-CLI/internal/format"
	keyringstore "github.com/Lensi-zhang/Akey-CLI/internal/keyring"
)

type application struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	config      *config.Store
	secrets     keyringstore.Store
	secretsErr  error
	secretsOnce sync.Once
	agents      *agent.Registry

	profileName    string
	output         string
	noColor        bool
	nonInteractive bool
	verbose        bool
	printer        *printer
}

func newApplication(stdin io.Reader, stdout, stderr io.Writer) (*application, error) {
	application := &application{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		config: config.NewStore(),
		agents: agent.NewDefaultRegistry(),
	}
	application.refreshPrinter()
	return application, nil
}

// secretStore 延迟打开系统密钥环，使不依赖凭证的命令仍可独立使用。
func (application *application) secretStore() (keyringstore.Store, error) {
	if application.secrets != nil {
		return application.secrets, nil
	}
	application.secretsOnce.Do(func() {
		application.secrets, application.secretsErr = keyringstore.NewSystem()
	})
	if application.secretsErr != nil {
		return nil, application.secretsErr
	}
	if application.secrets == nil {
		return nil, apperr.NewAuth("keyring_unavailable", "系统密钥环不可用", "请确认系统凭证服务已启用并解锁")
	}
	return application.secrets, nil
}

func (application *application) refreshPrinter() {
	application.printer = newPrinter(
		application.stdout,
		application.stderr,
		application.output == "json",
		application.noColor,
		application.verbose,
	)
}

func (application *application) selectedProfile() (config.Profile, error) {
	if application.profileName != "" {
		return application.config.Get(application.profileName)
	}
	return application.config.Current()
}

func (application *application) readSecret(profile config.Profile) (string, error) {
	store, err := application.secretStore()
	if err != nil {
		return "", err
	}
	return store.Get(profile)
}

func (application *application) emit(value any, table string) error {
	if application.output == "json" {
		encoded, err := format.JSON(value)
		if err != nil {
			return fmt.Errorf("生成 JSON 输出失败: %w", err)
		}
		application.printer.data(encoded)
		return nil
	}
	application.printer.data(table)
	return nil
}

func (application *application) requireInteractive() error {
	if application.nonInteractive {
		return apperr.NewUsage("interactive_required", "非交互模式缺少必要参数", "请通过命令选项提供所需信息；密钥只能通过交互式隐藏输入录入")
	}
	return nil
}
