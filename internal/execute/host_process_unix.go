//go:build !windows

package execute

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type unixProcessTree struct{}

func configureHostProcessTree(command *exec.Cmd) (hostProcessTree, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &unixProcessTree{}, nil
}

func (tree *unixProcessTree) Attach(*os.Process) error { return nil }

func (tree *unixProcessTree) Cancel(process *os.Process) error {
	if process == nil {
		return errors.New("host process is not started")
	}
	if err := syscall.Kill(-process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func (tree *unixProcessTree) Close() error { return nil }
