package rerun

import (
	"os/exec"
)

type Command interface {
	Start() error
	Kill() error
	Wait() error
	PID() string
}

type command struct {
	cmd  *exec.Cmd
	args []string
}

func StartCommand(args ...string) (Command, error) {
	cmd := newPlatformCmd(args...)

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}
