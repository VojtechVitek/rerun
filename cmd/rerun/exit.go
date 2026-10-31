package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// describeExit renders a reaped generation's end, e.g. "status 3" or
// "signal: terminated". code is negative when the child did not exit on its own.
func describeExit(code int, err error) string {
	if code >= 0 {
		return fmt.Sprintf("status %d", code)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ProcessState != nil {
		return exitErr.ProcessState.String()
	}
	return "signal"
}

// exitLabel is the short exit a TUI row shows: `exit 0`, `exit 1`, `exit
// segv`. code is negative when the child was killed by a signal.
func exitLabel(code int, err error) string {
	if code >= 0 {
		return fmt.Sprintf("exit %d", code)
	}
	return "exit " + signalName(err)
}

// signalName shortens a wait status, e.g. "signal: segmentation fault" to
// "segv".
func signalName(err error) string {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ProcessState == nil {
		return "signal"
	}
	switch text := strings.TrimPrefix(exitErr.ProcessState.String(), "signal: "); text {
	case "segmentation fault":
		return "segv"
	case "terminated":
		return "term"
	case "interrupt":
		return "int"
	case "killed":
		return "kill"
	case "broken pipe":
		return "pipe"
	case "hangup":
		return "hup"
	case "aborted":
		return "abrt"
	default:
		return text
	}
}
