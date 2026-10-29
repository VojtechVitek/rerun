//go:build !windows

package main

import (
	"os/exec"
	"testing"
)

func TestDescribeExitFromProcess(t *testing.T) {
	cases := []struct {
		script string
		want   string
	}{
		{"exit 0", "status 0"},
		{"exit 7", "status 7"},
		{"kill -TERM $$", "signal SIGTERM"},
	}

	for _, tc := range cases {
		t.Run(tc.script, func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", tc.script)
			err := cmd.Run()

			code := -1
			if cmd.ProcessState != nil {
				code = cmd.ProcessState.ExitCode()
			}
			if got := describeExit(code, err); got != tc.want {
				t.Errorf("describeExit(%q) = %q, want %q", tc.script, got, tc.want)
			}
		})
	}
}
