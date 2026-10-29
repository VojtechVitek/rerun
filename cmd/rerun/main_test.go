package main

import (
	"bytes"
	"testing"
	"time"
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

func TestWaitCooldownHoldsRestart(t *testing.T) {
	const cooldown = 150 * time.Millisecond

	begin := time.Now()
	waitCooldown(cooldown, begin)
	if elapsed := time.Since(begin); elapsed < cooldown {
		t.Fatalf("waitCooldown returned after %v, want >= %v", elapsed, cooldown)
	}

	// An already-elapsed window must not sleep.
	begin = time.Now()
	waitCooldown(cooldown, time.Now().Add(-time.Second))
	if elapsed := time.Since(begin); elapsed > 50*time.Millisecond {
		t.Fatalf("waitCooldown slept %v on an elapsed window", elapsed)
	}

	begin = time.Now()
	waitCooldown(0, time.Now().Add(-time.Second))
	if elapsed := time.Since(begin); elapsed > 50*time.Millisecond {
		t.Fatalf("waitCooldown slept %v with cooldown 0", elapsed)
	}
}
