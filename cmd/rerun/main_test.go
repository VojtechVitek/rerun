package main

import (
	"bytes"
	"testing"
)

func TestWarnNoGitignore(t *testing.T) {
	var stderr bytes.Buffer
	warnNoGitignore(Args{NoGitignore: true}, &stderr)
	if got, want := stderr.String(), noGitignoreWarning+"\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}

	stderr.Reset()
	warnNoGitignore(Args{}, &stderr)
	if stderr.Len() != 0 {
		t.Fatalf("unexpected warning without the flag: %q", stderr.String())
	}
}
