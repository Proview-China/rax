//go:build linux

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type groupSignal uint8

const (
	groupSignalTerminate groupSignal = iota + 1
	groupSignalKill
)

func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalProcessGroup(process *os.Process, pid int, signal groupSignal) error {
	var native syscall.Signal
	switch signal {
	case groupSignalTerminate:
		native = syscall.SIGTERM
	case groupSignalKill:
		native = syscall.SIGKILL
	default:
		return ErrInvalidConfig
	}
	err := syscall.Kill(-pid, native)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func processGroupAlive(_ *os.Process, pid int) (bool, error) {
	err := syscall.Kill(-pid, 0)
	switch {
	case err == nil, errors.Is(err, syscall.EPERM):
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	default:
		return false, err
	}
}

func processStateSignal(state *os.ProcessState) string {
	if state == nil {
		return ""
	}
	status, ok := state.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return ""
	}
	return status.Signal().String()
}
