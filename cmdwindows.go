package rerun

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

type WindowsCmd struct {
	*command
}

var _ Command = &WindowsCmd{}

func NewWindowsCmd(args ...string) *WindowsCmd {
	return &WindowsCmd{command: &command{args: args}}
}

func (c *WindowsCmd) Start() error {
	cmd := exec.Command("cmd", append([]string{"/c"}, c.args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	return nil
}

func (c *WindowsCmd) Kill() error {
	pid := c.cmd.Process.Pid
	// https://stackoverflow.com/a/44551450
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(pid))
	return kill.Run()
}

func (c *WindowsCmd) Wait() error {
	// Wait for the process to finish.
	return c.cmd.Wait()
}

func (c *WindowsCmd) PID() string {
	if c.cmd != nil {
		return fmt.Sprintf("PID %v", c.cmd.Process.Pid)
	} else {
		return "PID unknown"
	}
}

func (c *WindowsCmd) String() string {
	return c.PID()
}
