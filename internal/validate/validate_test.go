package validate

import (
	"strings"
	"testing"
)

func TestProfileAndServerNames(t *testing.T) {
	t.Parallel()
	valid := []string{"a", "0", "volc-prod", "open_ai", "a" + strings.Repeat("b", 31)}
	invalid := []string{"", "A", "-prod", "_prod", "has space", "中文", "a" + strings.Repeat("b", 32)}
	for _, name := range valid {
		if err := ProfileName(name); err != nil {
			t.Errorf("ProfileName(%q) returned %v", name, err)
		}
		if err := ServerName(name); err != nil {
			t.Errorf("ServerName(%q) returned %v", name, err)
		}
	}
	for _, name := range invalid {
		if err := ProfileName(name); err == nil {
			t.Errorf("ProfileName(%q) unexpectedly succeeded", name)
		}
		if err := ServerName(name); err == nil {
			t.Errorf("ServerName(%q) unexpectedly succeeded", name)
		}
	}
}

func TestBaseURL(t *testing.T) {
	t.Parallel()
	valid := []string{
		"http://localhost:8080/api/v3",
		"https://example.com",
		"HTTPS://example.com/api/v3?region=cn-beijing",
	}
	for _, rawURL := range valid {
		if err := BaseURL(rawURL); err != nil {
			t.Errorf("BaseURL(%q) returned %v", rawURL, err)
		}
	}

	const fakeSecret = "FAKE_TEST_KEY_DO_NOT_USE_98c63d"
	invalid := []string{
		"",
		"/api/v3",
		"example.com/api/v3",
		"ftp://example.com/api",
		"https:///api/v3",
		"https://user:" + fakeSecret + "@example.com/api",
		"https://example.com/api?api_key=" + fakeSecret,
		"https://example.com/api?apiKey=" + fakeSecret,
		"https://example.com/api?access-token=" + fakeSecret,
		"https://example.com/api?client_secret=" + fakeSecret,
		"https://example.com/api?password=" + fakeSecret,
		"https://example.com/api?signature=" + fakeSecret,
		"https://example.com/api?key=" + fakeSecret,
		"https://example.com/api?region=cn;api_key=" + fakeSecret,
	}
	for _, rawURL := range invalid {
		err := BaseURL(rawURL)
		if err == nil {
			t.Errorf("BaseURL(%q) unexpectedly succeeded", rawURL)
			continue
		}
		if strings.Contains(err.Error(), fakeSecret) || (rawURL != "" && strings.Contains(err.Error(), rawURL)) {
			t.Errorf("BaseURL error leaked its input: %v", err)
		}
	}
}

func TestEnvPrefix(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"API", "_", "OPENAI_API", "A1", "_PRIVATE_2"} {
		if err := EnvPrefix(prefix); err != nil {
			t.Errorf("EnvPrefix(%q) returned %v", prefix, err)
		}
	}
	for _, prefix := range []string{"", "api", "1API", "OPEN-AI", "OPEN AI", "中文"} {
		if err := EnvPrefix(prefix); err == nil {
			t.Errorf("EnvPrefix(%q) unexpectedly succeeded", prefix)
		}
	}
}

func TestOutputAndShell(t *testing.T) {
	t.Parallel()
	for _, output := range []string{"table", "json"} {
		if err := Output(output); err != nil {
			t.Errorf("Output(%q) returned %v", output, err)
		}
	}
	for _, output := range []string{"", "JSON", "yaml"} {
		if err := Output(output); err == nil {
			t.Errorf("Output(%q) unexpectedly succeeded", output)
		}
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell", "cmd"} {
		if err := Shell(shell); err != nil {
			t.Errorf("Shell(%q) returned %v", shell, err)
		}
	}
	for _, shell := range []string{"", "sh", "PowerShell"} {
		if err := Shell(shell); err == nil {
			t.Errorf("Shell(%q) unexpectedly succeeded", shell)
		}
	}
}
