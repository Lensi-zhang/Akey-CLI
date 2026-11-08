package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestExitCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "success", err: nil, want: 0},
		{name: "runtime", err: NewRuntime("runtime", "runtime", ""), want: 1},
		{name: "usage", err: NewUsage("usage", "usage", ""), want: 2},
		{name: "auth", err: NewAuth("auth", "auth", ""), want: 3},
		{name: "not found", err: NewNotFound("not_found", "not found", ""), want: 4},
		{name: "conflict", err: NewConflict("conflict", "conflict", ""), want: 5},
		{name: "canceled", err: NewCanceled("canceled", "canceled", ""), want: 5},
		{name: "config", err: NewConfig("config", "config", ""), want: 6},
		{name: "interrupted", err: NewInterrupted("interrupted", "interrupted", ""), want: 130},
		{name: "ordinary error", err: errors.New("ordinary"), want: 1},
		{name: "wrapped", err: fmt.Errorf("outer: %w", NewAuth("auth", "safe", "")), want: 3},
		{name: "unknown kind", err: New(Kind("unknown"), "unknown", "unknown", ""), want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ExitCode(test.err); got != test.want {
				t.Fatalf("ExitCode() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestErrorSupportsUnwrapWithoutLeakingCause(t *testing.T) {
	t.Parallel()
	const fakeSecret = "FAKE_TEST_KEY_DO_NOT_USE_8f4a27"
	cause := errors.New("backend rejected " + fakeSecret)
	err := NewAuth("keyring_write_failed", "The system keyring could not save the key.", "Unlock the keyring and try again.", cause)

	if !errors.Is(err, cause) {
		t.Fatal("errors.Is did not find the wrapped cause")
	}
	if got := err.Error(); strings.Contains(got, fakeSecret) || strings.Contains(got, "backend rejected") {
		t.Fatalf("Error() exposed its cause: %q", got)
	}
	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("json.Marshal: %v", marshalErr)
	}
	jsonText := string(encoded)
	if strings.Contains(jsonText, fakeSecret) || strings.Contains(jsonText, "backend rejected") || strings.Contains(jsonText, "Err") {
		t.Fatalf("JSON exposed the cause: %s", jsonText)
	}
	if !strings.Contains(jsonText, `"code":"keyring_write_failed"`) || !strings.Contains(jsonText, `"message"`) || !strings.Contains(jsonText, `"hint"`) {
		t.Fatalf("JSON omitted safe error fields: %s", jsonText)
	}
}

func TestNewDefaultsCodeToKind(t *testing.T) {
	t.Parallel()
	err := New(KindUsage, "", "safe message", "")
	if err.Code != string(KindUsage) {
		t.Fatalf("Code = %q, want %q", err.Code, KindUsage)
	}
	if (*Error)(nil).ExitCode() != 0 {
		t.Fatal("nil Error exit code must be zero")
	}
}
