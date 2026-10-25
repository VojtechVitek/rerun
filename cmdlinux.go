package rerun

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

type LinuxCmd struct {
	*command
}

var _ Command = &LinuxCmd{}

func NewLinuxCmd(args ...string) *LinuxCmd {
	return &LinuxCmd{command: &command{args: args}}
}

func (c *LinuxCmd) Start() error {
	cmd := exec.Command("/bin/sh", append([]string{"-c"}, c.args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	return nil
}

func (c *LinuxCmd) Kill() error {
	pid := c.cmd.Process.Pid

	// Try to kill the whole process group (which we created via Setpgid: true), if possible.
	// This should kill the command process, all its children and grandchildren.
	if pgid, err := syscall.Getpgid(pid); err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}

	// Kill the process.
	// Note: The process group kill syscall sometimes fails on Linux, so let's just do both.
	err := syscall.Kill(-pid, syscall.SIGKILL)

	c.cmd.Process.Wait()

	return err
}

func (c *LinuxCmd) Wait() error {
	// Wait for the process to finish.
	return c.cmd.Wait()
}

func (c *LinuxCmd) PID() string {
	if c.cmd != nil {
		return fmt.Sprintf("PID %v", c.cmd.Process.Pid)
	} else {
		return "PID unknown"
	}
}

func (c *LinuxCmd) String() string {
	return c.PID()
}
