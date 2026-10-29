//go:build darwin

package rerun

import "time"

func newPlatformCmd(cmdline []string, grace time.Duration, opts ...Option) Command {
	return newCommand(cmdline, grace, opts...)
}
