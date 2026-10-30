package main

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"
)

func TestDescribeExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}

	cases := []struct {
		script string
		want   string
	}{
		{"exit 0", "status 0"},
		{"exit 3", "status 3"},
		{"kill -TERM $$", "signal: terminated"},
	}

	for _, tc := range cases {
		t.Run(tc.script, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", tc.script)
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

func TestDescribeExitUnknownSignal(t *testing.T) {
	if got := describeExit(-1, errors.New("boom")); got != "signal" {
		t.Errorf("describeExit(-1, err) = %q, want %q", got, "signal")
	}
}
