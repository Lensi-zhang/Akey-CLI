package process

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	helperModeEnv  = "AKEY_PROCESS_TEST_HELPER"
	helperValueEnv = "AKEY_PROCESS_TEST_VALUE"
	parentOnlyEnv  = "AKEY_PROCESS_TEST_PARENT_ONLY"
)

func TestRunUsesExplicitEnvironment(t *testing.T) {
	t.Setenv(parentOnlyEnv, "parent-only")
	var stdout, stderr bytes.Buffer
	code, err := Run(
		os.Args[0],
		helperArgs("env"),
		[]string{helperModeEnv + "=1", helperValueEnv + "=expected"},
		nil,
		&stdout,
		&stderr,
	)
	if err != nil || code != 0 {
		t.Fatal("helper process did not receive the exact environment")
	}
	if stdout.String() != "env-ok\n" || stderr.Len() != 0 {
		t.Fatal("helper process environment verification produced unexpected IO")
	}
	if os.Getenv(parentOnlyEnv) != "parent-only" {
		t.Fatal("Run modified the parent environment")
	}
}

func TestRunForwardsStandardIO(t *testing.T) {
	const input = "stdin payload with spaces ; & $()\n"
	var stdout, stderr bytes.Buffer
	code, err := Run(
		os.Args[0],
		helperArgs("io"),
		[]string{helperModeEnv + "=1"},
		strings.NewReader(input),
		&stdout,
		&stderr,
	)
	if err != nil || code != 0 {
		t.Fatal("helper process IO run failed")
	}
	if stdout.String() != input {
		t.Fatal("stdin was not forwarded to stdout intact")
	}
	if stderr.String() != "helper-stderr\n" {
		t.Fatal("stderr was not forwarded intact")
	}
}

func TestRunReturnsChildExitCode(t *testing.T) {
	code, err := Run(os.Args[0], helperArgs("exit-37"), []string{helperModeEnv + "=1"}, nil, nil, nil)
	if code != 37 {
		t.Fatal("Run did not return the child's exit code")
	}
	var exitError *ExitError
	if !errors.As(err, &exitError) {
		t.Fatal("Run did not classify an unsuccessful child exit")
	}
	if exitError.Code != 37 || exitError.ExitCode() != 37 {
		t.Fatal("ExitError does not preserve the child's exit code")
	}
}

func TestRunPreservesArgumentBoundaries(t *testing.T) {
	const argument = "spaces ; & $(not-a-command) ' \" literal"
	code, err := Run(
		os.Args[0],
		helperArgs("args", argument),
		[]string{helperModeEnv + "=1"},
		nil,
		nil,
		nil,
	)
	if err != nil || code != 0 {
		t.Fatal("Run did not preserve argument boundaries")
	}
}

func TestRunClassifiesStartErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-helper-binary")
	code, err := Run(missing, nil, []string{}, nil, nil, nil)
	if code != -1 {
		t.Fatal("Run returned an exit code for a process that did not start")
	}
	var startError *StartError
	if !errors.As(err, &startError) {
		t.Fatal("Run did not classify the start error")
	}
	if startError.Unwrap() == nil {
		t.Fatal("StartError did not preserve the operating system error")
	}
}

func TestProcessHelper(t *testing.T) {
	if os.Getenv(helperModeEnv) != "1" {
		return
	}

	arguments := os.Args
	for index, argument := range arguments {
		if argument == "--" {
			arguments = arguments[index+1:]
			break
		}
	}
	if len(arguments) == 0 {
		os.Exit(90)
	}

	switch arguments[0] {
	case "env":
		if os.Getenv(helperValueEnv) != "expected" || os.Getenv(parentOnlyEnv) != "" {
			os.Exit(91)
		}
		_, _ = io.WriteString(os.Stdout, "env-ok\n")
		os.Exit(0)
	case "io":
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(92)
		}
		_, _ = os.Stdout.Write(input)
		_, _ = io.WriteString(os.Stderr, "helper-stderr\n")
		os.Exit(0)
	case "exit-37":
		os.Exit(37)
	case "args":
		if len(arguments) != 2 || arguments[1] != "spaces ; & $(not-a-command) ' \" literal" {
			os.Exit(93)
		}
		os.Exit(0)
	default:
		os.Exit(94)
	}
}

func helperArgs(mode string, additional ...string) []string {
	arguments := []string{"-test.run=^TestProcessHelper$", "--", mode}
	return append(arguments, additional...)
}
