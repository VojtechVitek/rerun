package rerun

import (
	"fmt"
	"os"
	"os/exec"
)

// NOTE: not in use.. darwin seems to work on darwin and linux

type LinuxCmd struct {
	*command
}

var _ Command = &DarwinCmd{}

func NewLinuxCmd(args ...string) *LinuxCmd {
	return &LinuxCmd{command: &command{args: args}}
}

func (c *LinuxCmd) Start() error {
	cmd := exec.Command("/bin/sh", append([]string{"-c"}, c.args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	return nil
}

func (c *LinuxCmd) Kill() error {
	return c.cmd.Process.Kill()
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
