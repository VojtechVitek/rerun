//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// startAdHoc boots the ad-hoc path on a temp watch dir. The returned stop
// function shuts the run down once with SIGINT and returns its exit code.
func startAdHoc(t *testing.T, script string) (out *safeBuffer, stop func() int, dir, src string) {
	t.Helper()

	dir = t.TempDir()
	src = filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	codePath := filepath.Join(dir, "code")
	writeFile(t, codePath, "3")

	script = strings.ReplaceAll(script, "{code}", codePath)
	script = strings.ReplaceAll(script, "{dir}", dir)

	out = &safeBuffer{}
	signals := make(chan os.Signal, 2)
	exitCode := make(chan int, 1)
	go func() {
		exitCode <- runAdHoc(Args{
			Watch: []string{src},
			Cmd:   []string{script},
			Delay: 20 * time.Millisecond,
			Grace: 500 * time.Millisecond,
		}, out, signals)
	}()

	var once sync.Once
	code := -1
	stop = func() int {
		once.Do(func() {
			select {
			case signals <- syscall.SIGINT:
			default:
			}
			select {
			case code = <-exitCode:
			case <-time.After(5 * time.Second):
				t.Error("ad-hoc run did not stop after SIGINT")
			}
		})
		return code
	}
	t.Cleanup(func() { stop() })

	return out, stop, dir, src
}

// The command's own output is passed through raw: no target prefix, no
// reformatting, and the screen is cleared exactly once at startup.
func TestAdHocPassesOutputThroughRaw(t *testing.T) {
	out, stop, _, src := startAdHoc(t, "echo adhoc-marker; sleep 10")

	waitForOutput(t, out, "adhoc-marker", 3*time.Second)

	var rawLine bool
	for _, line := range strings.Split(out.String(), "\n") {
		rawLine = rawLine || line == "adhoc-marker"
	}
	if !rawLine {
		t.Fatalf("ad-hoc output line was prefixed or reformatted:\n%q", out.String())
	}
	if got := out.String(); !strings.Contains(got, clearTerminal) {
		t.Fatalf("ad-hoc run did not clear the screen:\n%q", got)
	}

	writeFile(t, filepath.Join(src, "a.txt"), "a")
	waitForOutput(t, out, "# 1 file changed (e.g. ", 5*time.Second)
	if got := out.String(); !strings.Contains(got, greenColor+"# 1 file changed (e.g. ") {
		t.Fatalf("change line is not green:\n%q", got)
	}

	if got := stop(); got != 0 {
		t.Errorf("exit code after SIGINT = %d, want 0", got)
	}
}

// A second signal skips the grace period and exits 130.
func TestAdHocSecondSignalExits130(t *testing.T) {
	out := &safeBuffer{}
	signals := make(chan os.Signal, 2)
	exitCode := make(chan int, 1)
	go func() {
		exitCode <- runAdHoc(Args{
			Watch: []string{t.TempDir()},
			// A child that ignores SIGTERM keeps the first kill busy for the
			// whole grace period, so the second signal always lands.
			Cmd:   []string{"trap '' TERM; echo ready; while true; do sleep 1; done"},
			Delay: 20 * time.Millisecond,
			Grace: 500 * time.Millisecond,
		}, out, signals)
	}()

	waitForOutput(t, out, "ready", 3*time.Second)
	signals <- syscall.SIGINT
	signals <- syscall.SIGINT

	select {
	case code := <-exitCode:
		if code != 130 {
			t.Errorf("exit code after two signals = %d, want 130", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ad-hoc run did not stop after two signals")
	}
}

// A self-exiting command is reported once per generation.
func TestRunAdHocReportsSelfExit(t *testing.T) {
	out, _, dir, src := startAdHoc(t, "exit $(cat {code} 2>/dev/null || echo 0)")

	waitForOutput(t, out, "exited (status 3)", 5*time.Second)

	// Every generation from here on exits 5, so a restart landing before the
	// watcher is ready only costs another poke.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "exited (status 5)") && time.Now().Before(deadline) {
		writeFile(t, filepath.Join(dir, "code"), "5")
		writeFile(t, filepath.Join(src, "change.txt"), "x")
		time.Sleep(100 * time.Millisecond)
	}
	waitForOutput(t, out, "exited (status 5)", time.Second)

	if got := strings.Count(out.String(), "exited (status 3)"); got != 1 {
		t.Fatalf("first generation reported %d times, want 1:\n%s", got, out.String())
	}
}

// The kill rerun performs before a restart is not reported as an exit.
func TestRunAdHocRestartKillIsNotReported(t *testing.T) {
	out, stop, dir, src := startAdHoc(t, "echo tick >> {dir}/marker; sleep 10")
	marker := filepath.Join(dir, "marker")

	waitForCount(t, marker, 1, 3*time.Second)
	writeFile(t, filepath.Join(src, "change.txt"), "x")
	waitForCount(t, marker, 2, 5*time.Second)

	if got := out.String(); strings.Contains(got, "exited (") {
		t.Fatalf("a killed generation was reported as exited:\n%s", got)
	}
	if got := stop(); got != 0 {
		t.Errorf("exit code after SIGINT = %d, want 0", got)
	}
}
