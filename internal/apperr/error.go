package apperr

import "errors"

// Kind 标识稳定的、面向用户的应用错误类别。
type Kind string

const (
	KindUsage       Kind = "usage"
	KindAuth        Kind = "auth"
	KindNotFound    Kind = "not_found"
	KindConflict    Kind = "conflict"
	KindConfig      Kind = "config"
	KindRuntime     Kind = "runtime"
	KindCanceled    Kind = "canceled"
	KindInterrupted Kind = "interrupted"
)

// Error 仅包含可安全展示的字段。Err 被明确排除在 JSON 和 Error 之外，
// 避免底层错误意外泄露凭证。
type Error struct {
	Kind    Kind   `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Err     error  `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func New(kind Kind, code, message, hint string, cause ...error) *Error {
	if code == "" {
		code = string(kind)
	}
	e := &Error{Kind: kind, Code: code, Message: message, Hint: hint}
	if len(cause) > 0 {
		e.Err = cause[0]
	}
	return e
}

func NewUsage(code, message, hint string, cause ...error) *Error {
	return New(KindUsage, code, message, hint, cause...)
}

func NewAuth(code, message, hint string, cause ...error) *Error {
	return New(KindAuth, code, message, hint, cause...)
}

func NewNotFound(code, message, hint string, cause ...error) *Error {
	return New(KindNotFound, code, message, hint, cause...)
}

func NewConflict(code, message, hint string, cause ...error) *Error {
	return New(KindConflict, code, message, hint, cause...)
}

func NewConfig(code, message, hint string, cause ...error) *Error {
	return New(KindConfig, code, message, hint, cause...)
}

func NewRuntime(code, message, hint string, cause ...error) *Error {
	return New(KindRuntime, code, message, hint, cause...)
}

func NewCanceled(code, message, hint string, cause ...error) *Error {
	return New(KindCanceled, code, message, hint, cause...)
}

func NewInterrupted(code, message, hint string, cause ...error) *Error {
	return New(KindInterrupted, code, message, hint, cause...)
}

func (e *Error) ExitCode() int {
	if e == nil {
		return 0
	}
	switch e.Kind {
	case KindUsage:
		return 2
	case KindAuth:
		return 3
	case KindNotFound:
		return 4
	case KindConflict, KindCanceled:
		return 5
	case KindConfig:
		return 6
	case KindInterrupted:
		return 130
	case KindRuntime:
		return 1
	default:
		return 1
	}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var applicationError *Error
	if errors.As(err, &applicationError) {
		return applicationError.ExitCode()
	}
	return 1
}
