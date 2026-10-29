//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startAdHoc boots runAdHoc on a temp watch dir. runAdHoc exits only on a
// signal, so the test leaves it running and only observes its output.
func startAdHoc(t *testing.T, script string) (out *safeBuffer, dir, src string) {
	t.Helper()

	dir = t.TempDir()
	src = filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(dir, "code")
	writeFile(t, code, "3")

	script = strings.ReplaceAll(script, "{code}", code)
	script = strings.ReplaceAll(script, "{dir}", dir)

	out = &safeBuffer{}
	go runAdHoc(Args{
		Watch: []string{src},
		Cmd:   []string{script},
		Delay: 20 * time.Millisecond,
		Grace: 500 * time.Millisecond,
	}, out)

	return out, dir, src
}

func TestRunAdHocReportsSelfExit(t *testing.T) {
	out, dir, src := startAdHoc(t, "exit $(cat {code} 2>/dev/null || echo 0)")

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

func TestRunAdHocRestartKillIsNotReported(t *testing.T) {
	out, dir, src := startAdHoc(t, "echo x >> {dir}/marker; sleep 2; exit $(cat {code} 2>/dev/null || echo 0)")

	waitForFile(t, filepath.Join(dir, "marker"), 2*time.Second)
	time.Sleep(200 * time.Millisecond)

	writeFile(t, filepath.Join(dir, "code"), "5")
	writeFile(t, filepath.Join(src, "change.txt"), "x")

	waitForOutput(t, out, "exited (status 5)", 5*time.Second)
	if strings.Contains(out.String(), "status 3") {
		t.Fatalf("a killed generation was reported as exited:\n%s", out.String())
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
