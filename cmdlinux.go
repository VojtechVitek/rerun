//go:build linux

package rerun

import "time"

func newPlatformCmd(cmdline []string, grace time.Duration) Command {
	return newCommand(cmdline, grace)
}
