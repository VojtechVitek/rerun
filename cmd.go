package rerun

import (
	"os/exec"
	"runtime"
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
	var cmd Command

	switch runtime.GOOS {
	case "windows":
		cmd = NewWindowsCmd(args...)

	case "darwin":
		cmd = NewDarwinCmd(args...)

	default:
		// assume everything else is unix-like
		cmd = NewLinuxCmd(args...)
	}

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}
