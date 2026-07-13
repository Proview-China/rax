//go:build !linux

package process

import (
	"errors"
	"os"
	"os/exec"
)

type groupSignal uint8

const (
	groupSignalTerminate groupSignal = iota + 1
	groupSignalKill
)

func configureProcessGroup(*exec.Cmd) {}

func signalProcessGroup(process *os.Process, _ int, signal groupSignal) error {
	if process == nil {
		return nil
	}
	var err error
	if signal == groupSignalKill {
		err = process.Kill()
	} else {
		err = process.Signal(os.Interrupt)
	}
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func processGroupAlive(process *os.Process, _ int) (bool, error) {
	if process == nil {
		return false, nil
	}
	err := process.Signal(os.Signal(nil))
	if errors.Is(err, os.ErrProcessDone) {
		return false, nil
	}
	return err == nil, err
}

func processStateSignal(*os.ProcessState) string { return "" }
