package main

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/golang-cz/rerun"
)

// exitDescription renders how the current generation ended, e.g. "status 3"
// or "signal SIGKILL".
func exitDescription(cmd rerun.Command) string {
	code, err := cmd.ExitState()
	return describeExit(code, err)
}

// describeExit turns a reaped wait result into a short human string. code is
// negative when the process did not exit on its own.
func describeExit(code int, err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if name := signalName(exitErr.ProcessState); name != "" {
			return "signal " + name
		}
	}
	if code < 0 {
		return "signal"
	}
	return fmt.Sprintf("status %d", code)
}
