package main

import (
	"errors"
	"os/exec"
	"testing"
)

func TestDescribeExitStatus(t *testing.T) {
	cases := []struct {
		name string
		code int
		err  error
		want string
	}{
		{"clean exit", 0, nil, "status 0"},
		{"non-zero exit", 3, &exec.ExitError{}, "status 3"},
		{"unknown signal", -1, errors.New("boom"), "signal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeExit(tc.code, tc.err); got != tc.want {
				t.Errorf("describeExit(%d, %v) = %q, want %q", tc.code, tc.err, got, tc.want)
			}
		})
	}
}
