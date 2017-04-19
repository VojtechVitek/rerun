package rerun

import (
	"os"
	"os/exec"
)

type Cmd struct {
	cmd *exec.Cmd
}

func Run(args ...string) (*Cmd, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	c := &Cmd{
		cmd: cmd,
	}

	if err := c.run(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Cmd) run() error {
	if err := c.cmd.Start(); err != nil {
		return err
	}
	return nil
}

func (c *Cmd) Restart() error {
	if err := c.cmd.Process.Kill(); err != nil {
		return err
	}
	return c.run()
}
