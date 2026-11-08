package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrinterJSONSuppressesDecoratedMessages(t *testing.T) {
	var stdout, stderr bytes.Buffer
	printer := newPrinter(&stdout, &stderr, true, false, false)
	printer.success("完成")
	printer.info("信息")
	printer.data("{}\n")
	if stdout.String() != "{}\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestPrinterPlainTextHasNoEscapeSequences(t *testing.T) {
	var stdout, stderr bytes.Buffer
	printer := newPrinter(&stdout, &stderr, false, false, false)
	printer.success("完成")
	if strings.Contains(stderr.String(), "\x1b") {
		t.Fatalf("unexpected escape sequence: %q", stderr.String())
	}
}
