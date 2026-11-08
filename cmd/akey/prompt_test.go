package main

import (
	"bytes"
	"testing"
)

func TestPromptLine(t *testing.T) {
	var output bytes.Buffer
	value, err := promptLine(bytes.NewBufferString("  volc-prod\n"), &output, "Profile: ")
	if err != nil {
		t.Fatal(err)
	}
	if value != "volc-prod" {
		t.Fatalf("unexpected value %q", value)
	}
}

func TestConfirm(t *testing.T) {
	for input, expected := range map[string]bool{"y\n": true, "yes\n": true, "no\n": false, "\n": false} {
		confirmed, err := confirm(bytes.NewBufferString(input), &bytes.Buffer{}, "继续？")
		if err != nil {
			t.Fatal(err)
		}
		if confirmed != expected {
			t.Fatalf("confirm(%q) = %v, want %v", input, confirmed, expected)
		}
	}
}
