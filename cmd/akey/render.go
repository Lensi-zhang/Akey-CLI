package main

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
)

type printer struct {
	stdout  io.Writer
	stderr  io.Writer
	json    bool
	color   bool
	verbose bool
}

func newPrinter(stdout, stderr io.Writer, jsonOutput, noColor, verbose bool) *printer {
	return &printer{
		stdout:  stdout,
		stderr:  stderr,
		json:    jsonOutput,
		color:   !jsonOutput && !noColor && isTerminal(stdout),
		verbose: verbose,
	}
}

func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (printer *printer) success(message string) {
	if printer.json {
		return
	}
	if printer.color {
		message = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓ " + message)
	} else {
		message = "✓ " + message
	}
	_, _ = fmt.Fprintln(printer.stderr, message)
}

func (printer *printer) info(message string) {
	if printer.json {
		return
	}
	_, _ = fmt.Fprintln(printer.stderr, "ℹ "+message)
}

func (printer *printer) data(text string) {
	_, _ = fmt.Fprint(printer.stdout, text)
}
