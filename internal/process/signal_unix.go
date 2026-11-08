//go:build !windows

package process

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func notifySignals(signals chan<- os.Signal) {
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
}

func forwardSignal(process *os.Process, received os.Signal) {
	sig, ok := received.(syscall.Signal)
	if !ok {
		return
	}
	_ = syscall.Kill(-process.Pid, sig)
}
