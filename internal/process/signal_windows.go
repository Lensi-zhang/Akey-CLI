//go:build windows

package process

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

const ctrlBreakEvent = 1

var generateConsoleCtrlEvent = syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent")

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func notifySignals(signals chan<- os.Signal) {
	signal.Notify(signals, os.Interrupt)
}

func forwardSignal(process *os.Process, _ os.Signal) {
	result, _, _ := generateConsoleCtrlEvent.Call(ctrlBreakEvent, uintptr(process.Pid))
	if result == 0 {
		// 控制台事件不可用时终止子进程，避免等待期间留下孤儿。
		_ = process.Kill()
	}
}
