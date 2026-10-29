//go:build !windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startRunner boots runTargets from a rerun.yml fixture in a temp cwd. The
// returned stop function shuts the runner down once and returns its exit code.
func startRunner(t *testing.T, yml string) (*safeBuffer, func() int, string) {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ConfigFileName), yml)
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	out := &safeBuffer{}
	signals := make(chan os.Signal, 2)
	code := make(chan int, 1)
	go func() { code <- runTargets(cfg.Targets, out, signals) }()

	var once sync.Once
	exitCode := -1
	stop := func() int {
		once.Do(func() {
			select {
			case signals <- syscall.SIGTERM:
			default:
			}
			select {
			case exitCode = <-code:
			case <-time.After(5 * time.Second):
				t.Error("runner did not stop after SIGTERM")
			}
		})
		return exitCode
	}
	t.Cleanup(func() { stop() })

	return out, stop, dir
}

func waitForOutput(t *testing.T, out *safeBuffer, sub string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), sub) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q; output:\n%s", sub, out.String())
}

func waitForCount(t *testing.T, path string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := -1
	for time.Now().Before(deadline) {
		last = countLines(path)
		if last == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d lines in %s, got %d", want, path, last)
}

func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "\n")
}

func TestRunTargetsParallelPrefixesOutput(t *testing.T) {
	out, stop, dir := startRunner(t, `a:
  on: src
  cmd: echo a > a.out && echo a-finished
b:
  on: src
  cmd: echo b > b.out && echo b-finished
`)

	waitForOutput(t, out, "[a] a-finished", 3*time.Second)
	waitForOutput(t, out, "[b] b-finished", 3*time.Second)
	waitForOutput(t, out, "[a] exited (status 0)", 3*time.Second)
	waitForOutput(t, out, "[b] exited (status 0)", 3*time.Second)

	if got := readFile(t, filepath.Join(dir, "a.out")); got != "a\n" {
		t.Errorf("a.out = %q, want %q", got, "a\n")
	}
	if got := readFile(t, filepath.Join(dir, "b.out")); got != "b\n" {
		t.Errorf("b.out = %q, want %q", got, "b\n")
	}

	if got := stop(); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

func TestRunTargetsSelfExitDoesNotRestart(t *testing.T) {
	out, stop, dir := startRunner(t, `t:
  on: src
  cmd: echo tick >> count.txt
`)

	count := filepath.Join(dir, "count.txt")
	waitForOutput(t, out, "[t] exited (status 0)", 3*time.Second)
	waitForCount(t, count, 1, 2*time.Second)

	// A self-exiting target must stay down until something changes.
	time.Sleep(300 * time.Millisecond)
	if got := countLines(count); got != 1 {
		t.Fatalf("target restarted on its own: %d ticks", got)
	}

	writeFile(t, filepath.Join(dir, "src", "change.txt"), "x")
	waitForCount(t, count, 2, 3*time.Second)

	if got := stop(); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

func TestRunTargetsReportsSignalExit(t *testing.T) {
	out, stop, _ := startRunner(t, `t:
  on: src
  cmd: kill -TERM $$
`)

	waitForOutput(t, out, "[t] exited (signal SIGTERM)", 3*time.Second)

	if got := stop(); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

func TestRunTargetsCooldown(t *testing.T) {
	_, stop, dir := startRunner(t, `t:
  on: src
  cmd: echo tick >> count.txt; sleep 5
  delay: 20ms
  cooldown: 900ms
  grace: 200ms
`)

	count := filepath.Join(dir, "count.txt")
	waitForCount(t, count, 1, 3*time.Second)

	writeFile(t, filepath.Join(dir, "src", "a.txt"), "a")
	waitForCount(t, count, 2, 3*time.Second)

	// These changes must wait out the cooldown before restarting.
	writeFile(t, filepath.Join(dir, "src", "b.txt"), "b")
	writeFile(t, filepath.Join(dir, "src", "c.txt"), "c")
	writeFile(t, filepath.Join(dir, "src", "d.txt"), "d")

	time.Sleep(300 * time.Millisecond)
	if got := countLines(count); got != 2 {
		t.Errorf("cooldown not honoured: %d ticks after 300ms, want 2", got)
	}

	waitForCount(t, count, 3, 3*time.Second)

	if got := stop(); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
