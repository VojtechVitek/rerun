//go:build windows

package rerun

import (
	"os"
	"os/exec"
	"strconv"
	"time"
)

func newPlatformCmd(cmdline []string, grace time.Duration) Command {
	return newCommand(cmdline, grace)
}

func newExecCmd(cmdline []string) *exec.Cmd {
	cmd := exec.Command("cmd", append([]string{"/c"}, cmdline...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd
}

// signalGroup: Windows has no signal a console child can handle gracefully, so
// both the graceful and the forced pass are the same tree kill.
func signalGroup(cmd *exec.Cmd, force bool) error {
	if cmd.Process == nil {
		return nil
	}
	// https://stackoverflow.com/a/44551450
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	return kill.Run()
}
