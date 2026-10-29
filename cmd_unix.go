//go:build darwin || linux

package rerun

import (
	"os"
	"os/exec"
	"syscall"
)

func newExecCmd(cmdline []string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", append([]string{"-c"}, cmdline...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	// Own process group, so one signal reaches the shell and everything it spawned.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

func signalGroup(cmd *exec.Cmd, force bool) error {
	if cmd.Process == nil {
		return nil
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}

	pid := cmd.Process.Pid
	err := syscall.Kill(-pid, sig)

	// The child may have moved itself into another group; cover that too.
	if pgid, perr := syscall.Getpgid(pid); perr == nil && pgid != pid {
		err = syscall.Kill(-pgid, sig)
	}
	return err
}
