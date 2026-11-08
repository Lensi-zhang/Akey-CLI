package envinject

import (
	"errors"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/config"
)

var (
	ErrUnsupportedShell = errors.New("unsupported shell")
	ErrInvalidPrefix    = errors.New("invalid environment variable prefix")
	ErrInvalidValue     = errors.New("environment variable value contains an unsupported character")
)

// Export 仅返回导出 Profile 变量所需的命令脚本。
func Export(shell string, profile config.Profile, key string) (string, error) {
	prefix := prefixOrDefault(profile.EnvPrefix)
	if !validPrefix(prefix) {
		return "", ErrInvalidPrefix
	}
	if !supportedShell(shell) {
		return "", ErrUnsupportedShell
	}
	for _, value := range []string{key, profile.BaseURL, profile.Model} {
		if strings.ContainsRune(value, '\x00') || (shell == "cmd" && strings.ContainsAny(value, "\r\n%!")) {
			return "", ErrInvalidValue
		}
	}

	keyName, baseURLName, modelName := variableNames(prefix)
	names := []string{keyName, baseURLName, modelName}
	values := []string{key, profile.BaseURL, profile.Model}

	var script strings.Builder
	for i, name := range names {
		switch shell {
		case "bash", "zsh":
			script.WriteString("export ")
			script.WriteString(name)
			script.WriteString("=")
			script.WriteString(quotePOSIX(values[i]))
		case "fish":
			script.WriteString("set -gx ")
			script.WriteString(name)
			script.WriteString(" ")
			script.WriteString(quoteFish(values[i]))
		case "powershell":
			script.WriteString("$env:")
			script.WriteString(name)
			script.WriteString(" = ")
			script.WriteString(quotePowerShell(values[i]))
		case "cmd":
			script.WriteString("set ")
			script.WriteString(name)
			script.WriteString("=")
			script.WriteString(escapeCMD(values[i]))
		}
		script.WriteByte('\n')
	}
	return script.String(), nil
}

// Unset 仅返回清除三个变量所需的命令脚本。
func Unset(shell, prefix string) (string, error) {
	prefix = prefixOrDefault(prefix)
	if !validPrefix(prefix) {
		return "", ErrInvalidPrefix
	}
	if !supportedShell(shell) {
		return "", ErrUnsupportedShell
	}

	keyName, baseURLName, modelName := variableNames(prefix)
	var script strings.Builder
	for _, name := range []string{keyName, baseURLName, modelName} {
		switch shell {
		case "bash", "zsh":
			script.WriteString("unset ")
			script.WriteString(name)
		case "fish":
			script.WriteString("set -e ")
			script.WriteString(name)
		case "powershell":
			script.WriteString("$env:")
			script.WriteString(name)
			script.WriteString(" = $null")
		case "cmd":
			script.WriteString("set ")
			script.WriteString(name)
			script.WriteString("=")
		}
		script.WriteByte('\n')
	}
	return script.String(), nil
}

func supportedShell(shell string) bool {
	switch shell {
	case "bash", "zsh", "fish", "powershell", "cmd":
		return true
	default:
		return false
	}
}

func validPrefix(prefix string) bool {
	if prefix == "" || !isUpperLetter(prefix[0]) && prefix[0] != '_' {
		return false
	}
	for i := 1; i < len(prefix); i++ {
		if !isUpperLetter(prefix[i]) && (prefix[i] < '0' || prefix[i] > '9') && prefix[i] != '_' {
			return false
		}
	}
	return true
}

func isUpperLetter(char byte) bool {
	return char >= 'A' && char <= 'Z'
}

func quotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func quoteFish(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "'", "\\'")
	return "'" + value + "'"
}

func quotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// CMD 输出面向交互式会话；百分号和延迟展开字符由 Export 明确拒绝。
// 插入符转义可确保双引号和命令分隔符均按字面量处理。
func escapeCMD(value string) string {
	var escaped strings.Builder
	escaped.Grow(len(value))
	for _, char := range []byte(value) {
		switch char {
		case '^', '&', '|', '<', '>', '(', ')', '"':
			escaped.WriteByte('^')
			escaped.WriteByte(char)
		default:
			escaped.WriteByte(char)
		}
	}
	return escaped.String()
}
