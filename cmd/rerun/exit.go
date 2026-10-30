package main

import (
	"errors"
	"fmt"
	"os/exec"
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
