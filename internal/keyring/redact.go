package keyring

import (
	"errors"

	systemkeyring "github.com/99designs/keyring"
)

var (
	ErrNotConfigured = errors.New("key not configured")
	ErrUnavailable   = errors.New("system keyring unavailable")
)

func errorsIsNotFound(err error) bool {
	return err != nil && (errors.Is(err, systemkeyring.ErrKeyNotFound) || errors.Is(err, ErrNotConfigured))
}

// redactedCause 不透传第三方错误文本，从错误链中彻底移除可能包含密钥的内容。
func redactedCause(err error) error {
	if err == nil {
		return nil
	}
	if errorsIsNotFound(err) {
		return ErrNotConfigured
	}
	return ErrUnavailable
}
