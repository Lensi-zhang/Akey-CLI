package envinject

import (
	"errors"
	"strings"
	"testing"

	"github.com/Lensi-zhang/Akey-CLI/internal/config"
)

func TestExportEscapesValuesAndWritesOnlyScript(t *testing.T) {
	tests := []struct {
		name        string
		shell       string
		value       string
		wantValue   string
		assignments []string
	}{
		{
			name:        "bash",
			shell:       "bash",
			value:       "a'b\n$(touch should-not-run) ; &",
			wantValue:   "'a'\\''b\n$(touch should-not-run) ; &'",
			assignments: []string{"export API_KEY=", "export API_BASE_URL=", "export API_MODEL="},
		},
		{
			name:        "zsh",
			shell:       "zsh",
			value:       "a'b\n`touch should-not-run` ; &",
			wantValue:   "'a'\\''b\n`touch should-not-run` ; &'",
			assignments: []string{"export API_KEY=", "export API_BASE_URL=", "export API_MODEL="},
		},
		{
			name:        "fish",
			shell:       "fish",
			value:       "a'b\\c\n$(touch should-not-run) ; &",
			wantValue:   "'a\\'b\\\\c\n$(touch should-not-run) ; &'",
			assignments: []string{"set -gx API_KEY ", "set -gx API_BASE_URL ", "set -gx API_MODEL "},
		},
		{
			name:        "powershell",
			shell:       "powershell",
			value:       "a'b\n$(Invoke-Expression bad); &",
			wantValue:   "'a''b\n$(Invoke-Expression bad); &'",
			assignments: []string{"$env:API_KEY = ", "$env:API_BASE_URL = ", "$env:API_MODEL = "},
		},
		{
			name:        "cmd",
			shell:       "cmd",
			value:       "a&b|c<d>e(f)g^h\"i",
			wantValue:   "a^&b^|c^<d^>e^(f^)g^^h^\"i",
			assignments: []string{"set API_KEY=", "set API_BASE_URL=", "set API_MODEL="},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := config.Profile{BaseURL: test.value, Model: test.value}
			got, err := Export(test.shell, profile, test.value)
			if err != nil {
				t.Fatal("Export rejected a supported value")
			}
			var want strings.Builder
			for _, assignment := range test.assignments {
				want.WriteString(assignment)
				want.WriteString(test.wantValue)
				want.WriteByte('\n')
			}
			if got != want.String() {
				t.Fatal("Export produced incorrect escaping or extra output")
			}
		})
	}
}

func TestExportUsesCustomPrefix(t *testing.T) {
	got, err := Export("bash", config.Profile{EnvPrefix: "CUSTOM_API"}, "fake-key")
	if err != nil {
		t.Fatal("Export failed for a valid custom prefix")
	}
	want := "export CUSTOM_API_KEY='fake-key'\n" +
		"export CUSTOM_API_BASE_URL=''\n" +
		"export CUSTOM_API_MODEL=''\n"
	if got != want {
		t.Fatal("Export did not use the custom prefix")
	}
}

func TestUnsetWritesOnlyUnsetScript(t *testing.T) {
	tests := []struct {
		shell string
		want  string
	}{
		{shell: "bash", want: "unset API_KEY\nunset API_BASE_URL\nunset API_MODEL\n"},
		{shell: "zsh", want: "unset API_KEY\nunset API_BASE_URL\nunset API_MODEL\n"},
		{shell: "fish", want: "set -e API_KEY\nset -e API_BASE_URL\nset -e API_MODEL\n"},
		{shell: "powershell", want: "$env:API_KEY = $null\n$env:API_BASE_URL = $null\n$env:API_MODEL = $null\n"},
		{shell: "cmd", want: "set API_KEY=\nset API_BASE_URL=\nset API_MODEL=\n"},
	}
	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			got, err := Unset(test.shell, "")
			if err != nil {
				t.Fatal("Unset rejected a supported shell")
			}
			if got != test.want {
				t.Fatal("Unset produced an incorrect script")
			}
		})
	}

	got, err := Unset("bash", "CUSTOM_API")
	if err != nil {
		t.Fatal("Unset rejected a valid custom prefix")
	}
	if got != "unset CUSTOM_API_KEY\nunset CUSTOM_API_BASE_URL\nunset CUSTOM_API_MODEL\n" {
		t.Fatal("Unset did not use the custom prefix")
	}
}

func TestExportRejectsInvalidInputWithoutLeakingValues(t *testing.T) {
	const fakeKey = "fake-key-that-must-not-appear-in-errors"
	tests := []struct {
		name    string
		shell   string
		profile config.Profile
		key     string
		wantErr error
	}{
		{name: "shell", shell: "bash;echo bad", key: fakeKey, wantErr: ErrUnsupportedShell},
		{name: "prefix", shell: "bash", profile: config.Profile{EnvPrefix: "API;echo bad"}, key: fakeKey, wantErr: ErrInvalidPrefix},
		{name: "nul", shell: "powershell", key: fakeKey + "\x00", wantErr: ErrInvalidValue},
		{name: "cmd newline", shell: "cmd", key: fakeKey + "\necho bad", wantErr: ErrInvalidValue},
		{name: "cmd carriage return", shell: "cmd", key: fakeKey + "\recho bad", wantErr: ErrInvalidValue},
		{name: "cmd percent expansion", shell: "cmd", key: fakeKey + "%PATH%", wantErr: ErrInvalidValue},
		{name: "cmd delayed expansion", shell: "cmd", key: fakeKey + "!value!", wantErr: ErrInvalidValue},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script, err := Export(test.shell, test.profile, test.key)
			if !errors.Is(err, test.wantErr) {
				t.Fatal("Export returned the wrong error category")
			}
			if script != "" {
				t.Fatal("Export returned a partial script after an error")
			}
			if strings.Contains(err.Error(), fakeKey) {
				t.Fatal("Export error contains the key")
			}
		})
	}
}

func TestUnsetRejectsInvalidInput(t *testing.T) {
	if script, err := Unset("unknown", "API"); !errors.Is(err, ErrUnsupportedShell) || script != "" {
		t.Fatal("Unset did not reject an unsupported shell")
	}
	if script, err := Unset("bash", "bad-prefix"); !errors.Is(err, ErrInvalidPrefix) || script != "" {
		t.Fatal("Unset did not reject an invalid prefix")
	}
}
