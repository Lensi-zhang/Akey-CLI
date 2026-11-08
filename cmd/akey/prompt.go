package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Lensi-zhang/Akey-CLI/internal/apperr"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	errInputCanceled = errors.New("输入已取消")
	errEmptySecret   = errors.New("密钥不能为空")
)

// readLine 不预读后续行，确保连续提示可以共享同一个输入流。
func readLine(in io.Reader, out io.Writer, label string) (string, error) {
	if _, err := fmt.Fprint(out, label); err != nil {
		return "", err
	}
	var value strings.Builder
	var buffer [1]byte
	for {
		count, err := in.Read(buffer[:])
		if count > 0 {
			if buffer[0] == '\n' {
				break
			}
			value.WriteByte(buffer[0])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
	}
	return strings.TrimSpace(value.String()), nil
}

func promptLine(in io.Reader, out io.Writer, label string) (string, error) {
	value, err := readLine(in, out, label)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", apperr.NewUsage("input_required", "输入不能为空", "请提供非空值")
	}
	return value, nil
}

func confirm(in io.Reader, out io.Writer, message string) (bool, error) {
	value, err := readLine(in, out, message+" [y/N] ")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(value) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

type secretModel struct {
	label    string
	value    []rune
	canceled bool
}

func (model secretModel) Init() tea.Cmd { return nil }

func (model secretModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch key.Type {
	case tea.KeyEnter:
		return model, tea.Quit
	case tea.KeyCtrlC, tea.KeyEsc:
		model.canceled = true
		return model, tea.Quit
	case tea.KeyBackspace, tea.KeyDelete:
		if len(model.value) > 0 {
			model.value = model.value[:len(model.value)-1]
		}
	case tea.KeyRunes:
		model.value = append(model.value, key.Runes...)
	}
	return model, nil
}

func (model secretModel) View() string {
	return model.label + strings.Repeat("*", len(model.value))
}

func promptSecret(in io.Reader, out io.Writer, label string) (string, error) {
	program := tea.NewProgram(
		secretModel{label: label},
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	result, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("读取密钥输入失败: %w", err)
	}
	model := result.(secretModel)
	if model.canceled {
		return "", errInputCanceled
	}
	if len(model.value) == 0 {
		return "", errEmptySecret
	}
	_, _ = fmt.Fprintln(out)
	return string(model.value), nil
}
