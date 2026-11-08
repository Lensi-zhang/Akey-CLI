package validate

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"
)

var (
	namePattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	envPrefixPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

func ProfileName(name string) error {
	if namePattern.MatchString(name) {
		return nil
	}
	return apperr.NewConfig(
		"invalid_profile_name",
		"Profile name is invalid.",
		"Use 1-32 lowercase letters, digits, underscores, or hyphens; the first character must be a letter or digit.",
	)
}

func ServerName(name string) error {
	if namePattern.MatchString(name) {
		return nil
	}
	return apperr.NewConfig(
		"invalid_server_name",
		"Server name is invalid.",
		"Use 1-32 lowercase letters, digits, underscores, or hyphens; the first character must be a letter or digit.",
	)
}

func BaseURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || !u.IsAbs() || u.Host == "" || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) || u.User != nil {
		return invalidBaseURL()
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return invalidBaseURL()
	}
	for key := range query {
		if isCredentialQueryKey(key) {
			return invalidBaseURL()
		}
	}
	return nil
}

func invalidBaseURL() error {
	return apperr.NewConfig(
		"invalid_base_url",
		"Base URL is invalid or contains credentials.",
		"Use an absolute http or https URL without user information or credential query parameters.",
	)
}

func isCredentialQueryKey(key string) bool {
	normalized := strings.ToLower(key)
	normalized = strings.NewReplacer("_", "", "-", "", ".", "").Replace(normalized)
	if normalized == "key" || normalized == "apikey" || normalized == "auth" || normalized == "authorization" || normalized == "passwd" || normalized == "pwd" {
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "signature")
}

func EnvPrefix(prefix string) error {
	if envPrefixPattern.MatchString(prefix) {
		return nil
	}
	return apperr.NewConfig(
		"invalid_env_prefix",
		"Environment variable prefix is invalid.",
		"Use uppercase letters, digits, and underscores; the first character must be an uppercase letter or underscore.",
	)
}

func Output(output string) error {
	if output == "table" || output == "json" {
		return nil
	}
	return apperr.NewUsage("invalid_output", "Output format is invalid.", "Use table or json.")
}

func Shell(shell string) error {
	switch shell {
	case "bash", "zsh", "fish", "powershell", "cmd":
		return nil
	default:
		return apperr.NewUsage("invalid_shell", "Shell is invalid.", "Use bash, zsh, fish, powershell, or cmd.")
	}
}
