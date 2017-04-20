package rerun

import (
	"os"
	"os/exec"
	"syscall"
)

type Cmd struct {
	cmd  *exec.Cmd
	args []string
}

func StartCommand(args ...string) (*Cmd, error) {
	c := &Cmd{
		args: args,
	}
	if err := c.Start(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Cmd) Start() error {
	cmd := exec.Command(c.args[0], c.args[1:]...)
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

func (c *Cmd) Kill() error {
	// Kill the children process group, which we created via Setpgid: true.
	// This should kill children and all its children.
	if pgid, err := syscall.Getpgid(c.cmd.Process.Pid); err == nil {
		syscall.Kill(-pgid, 9)
	}

	// Make sure our own children gets killed.
	if err := c.cmd.Process.Kill(); err != nil {
		return err
	}

	// Wait for the children to finish.
	if err := c.cmd.Wait(); err != nil {
		return err
	}

	return nil
}

func (c *Cmd) String() string {
	str := "$"
	for _, arg := range c.args {
		str += " " + arg
	}
	return str
}
