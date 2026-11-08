package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
)

// StartError 表示子进程无法启动。
type StartError struct {
	Err error
}

func (e *StartError) Error() string {
	return "start child process: " + e.Err.Error()
}

func (e *StartError) Unwrap() error {
	return e.Err
}

// ExitError 表示已启动的子进程未成功退出。
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Code >= 0 {
		return fmt.Sprintf("child process exited with code %d", e.Code)
	}
	return "child process terminated without an exit code"
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func (e *ExitError) ExitCode() int {
	return e.Code
}

// WaitError 表示无法取得子进程退出状态。
type WaitError struct {
	Err error
}

func (e *WaitError) Error() string {
	return "wait for child process: " + e.Err.Error()
}

func (e *WaitError) Unwrap() error {
	return e.Err
}

// Run 直接启动命令、等待其结束并返回退出码。
// 返回码为 -1 表示无法取得子进程退出码。
func Run(command string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append([]string{}, env...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	configureProcess(cmd)

	signals := make(chan os.Signal, 2)
	notifySignals(signals)
	defer signal.Stop(signals)

	if err := cmd.Start(); err != nil {
		return -1, &StartError{Err: err}
	}

	done := make(chan struct{})
	forwarderDone := make(chan struct{})
	go func() {
		defer close(forwarderDone)
		for {
			select {
			case received := <-signals:
				forwardSignal(cmd.Process, received)
			case <-done:
				return
			}
		}
	}()

	err := cmd.Wait()
	signal.Stop(signals)
	close(done)
	<-forwarderDone

	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		code := exitError.ExitCode()
		return code, &ExitError{Code: code, Err: err}
	}
	return -1, &WaitError{Err: err}
}
