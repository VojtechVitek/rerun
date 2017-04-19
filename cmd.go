package rerun

import (
	"os"
	"os/exec"
)

type Cmd struct {
	cmd  *exec.Cmd
	args []string
}

func Run(args ...string) (*Cmd, error) {
	cmd := &Cmd{
		args: args,
	}
	if err := cmd.run(); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (c *Cmd) run() error {
	c.cmd = nil

	cmd := exec.Command(c.args[0], c.args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd

	return nil
}

func (c *Cmd) Restart() error {
	if c.cmd != nil {
		if err := c.cmd.Process.Kill(); err != nil {
			return err
		}
	}
	return c.run()
}
