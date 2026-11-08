package format

import (
	"strings"
	"testing"
)

func TestTable(t *testing.T) {
	output := Table([]string{"NAME", "MODEL"}, [][]string{{"default", "model-a"}})
	if !strings.Contains(output, "NAME") || !strings.Contains(output, "model-a") {
		t.Fatalf("unexpected table: %q", output)
	}
	if strings.Contains(output, "\x1b") {
		t.Fatal("table contains escape sequences")
	}
}

func TestTableSanitizesControlCharacters(t *testing.T) {
	output := Table([]string{"NOTE"}, [][]string{{"safe\x1b[31m\nnext"}})
	if strings.ContainsAny(output, "\x1b\r") || strings.Contains(output, "next\n") == false {
		t.Fatalf("unexpected sanitized output: %q", output)
	}
}

func TestJSON(t *testing.T) {
	output, err := JSON(map[string]string{"name": "default"})
	if err != nil {
		t.Fatal(err)
	}
	if output != "{\n  \"name\": \"default\"\n}\n" {
		t.Fatalf("unexpected JSON: %q", output)
	}
}
