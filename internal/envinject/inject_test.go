package envinject

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Lensi-zhang/Akey-CLI/internal/config"
)

func TestBuildReplacesVariablesWithoutAliasingBase(t *testing.T) {
	base := make([]string, 0, 16)
	base = append(base,
		"PATH=keep",
		"API_KEY=old-one",
		"api_key=unix-value",
		"API_KEY=old-two",
		"API_BASE_URL=old-base",
		"API_MODEL=old-model",
		"MALFORMED",
		"=C:=C:\\keep",
	)
	original := slices.Clone(base)
	profile := config.Profile{BaseURL: "https://example.invalid", Model: "fake-model"}

	got := build(base, profile, "fake-key", false)

	if !slices.Equal(base, original) {
		t.Fatal("Build modified its input environment")
	}
	if countName(got, "API_KEY", false) != 1 || countName(got, "API_BASE_URL", false) != 1 || countName(got, "API_MODEL", false) != 1 {
		t.Fatal("Build did not remove all old target variables")
	}
	if countName(got, "api_key", false) != 1 {
		t.Fatal("Build removed a case-distinct variable on Unix")
	}
	if value, ok := lookup(got, "API_KEY", false); !ok || value != "fake-key" {
		t.Fatal("Build did not inject the key")
	}
	if value, ok := lookup(got, "API_BASE_URL", false); !ok || value != profile.BaseURL {
		t.Fatal("Build did not inject the base URL")
	}
	if value, ok := lookup(got, "API_MODEL", false); !ok || value != profile.Model {
		t.Fatal("Build did not inject the model")
	}
	if !slices.Contains(got, "MALFORMED") || !slices.Contains(got, "=C:=C:\\keep") {
		t.Fatal("Build did not preserve unrelated entries")
	}

	got[0] = "PATH=changed"
	if base[0] != "PATH=keep" {
		t.Fatal("Build returned storage aliased to its input")
	}
}

func TestBuildUsesWindowsCaseInsensitiveNames(t *testing.T) {
	base := []string{
		"custom_api_key=old-one",
		"CUSTOM_API_Key=old-two",
		"custom_api_base_url=old-base",
		"Custom_Api_Model=old-model",
		"other_api_key=keep",
	}
	profile := config.Profile{EnvPrefix: "CUSTOM_API", BaseURL: "base", Model: "model"}
	got := build(base, profile, "fake-key", true)

	for _, name := range []string{"CUSTOM_API_KEY", "CUSTOM_API_BASE_URL", "CUSTOM_API_MODEL"} {
		if countName(got, name, true) != 1 {
			t.Fatalf("Build left duplicate Windows variables for %s", name)
		}
	}
	if countName(got, "OTHER_API_KEY", true) != 1 {
		t.Fatal("Build removed an unrelated Windows variable")
	}
}

func TestBuildUsesDefaultAndCustomPrefixes(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{name: "default", want: "API"},
		{name: "custom", prefix: "OPENAI_API", want: "OPENAI_API"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Build(nil, config.Profile{EnvPrefix: test.prefix}, "fake-key")
			if len(got) != 3 {
				t.Fatal("Build did not return exactly three variables")
			}
			for _, suffix := range []string{"_KEY", "_BASE_URL", "_MODEL"} {
				if countName(got, test.want+suffix, false) != 1 {
					t.Fatalf("Build did not generate %s", test.want+suffix)
				}
			}
		})
	}
}

func TestEnvironmentFunctionsDoNotChangeParentEnvironment(t *testing.T) {
	const prefix = "AKEY_ENVINJECT_PARENT_TEST"
	t.Setenv(prefix+"_KEY", "parent-value")
	profile := config.Profile{EnvPrefix: prefix, BaseURL: "https://example.invalid", Model: "fake-model"}

	_ = Build(os.Environ(), profile, "fake-key")
	if _, err := Export("bash", profile, "fake-key"); err != nil {
		t.Fatal("Export failed")
	}
	if _, err := Unset("bash", prefix); err != nil {
		t.Fatal("Unset failed")
	}

	if os.Getenv(prefix+"_KEY") != "parent-value" {
		t.Fatal("an environment helper modified the parent process")
	}
}

func countName(environment []string, name string, fold bool) int {
	count := 0
	for _, entry := range environment {
		entryName, _, _ := strings.Cut(entry, "=")
		if sameName(entryName, name, fold) {
			count++
		}
	}
	return count
}

func lookup(environment []string, name string, fold bool) (string, bool) {
	for _, entry := range environment {
		entryName, value, found := strings.Cut(entry, "=")
		if found && sameName(entryName, name, fold) {
			return value, true
		}
	}
	return "", false
}
