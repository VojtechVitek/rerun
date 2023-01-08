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
	// case "darwin", "ios":
	// cmd = NewDarwinCmd(args...)

	case "windows":
		cmd = NewWindowsCmd(args...)

	default:
		// assume everything else is linux
		cmd = NewDarwinCmd(args...)
		// cmd = NewLinuxCmd(args...)
	}

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}
