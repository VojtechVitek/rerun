package rerun

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFiles(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExpandGlobPaths(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a_test.go")
	b := filepath.Join(dir, "b.go")
	writeFiles(t, a, b)

	got := expandGlobPaths(filepath.Join(dir, "*_test.go"))
	if len(got) != 1 || got[0] != a {
		t.Fatalf("glob = %v, want [%s]", got, a)
	}

	got = expandGlobPaths(b)
	if len(got) != 1 || got[0] != b {
		t.Fatalf("non-glob = %v, want [%s]", got, b)
	}

	if got := expandGlobPaths(filepath.Join(dir, "*.nomatch")); len(got) != 0 {
		t.Fatalf("no match = %v, want empty", got)
	}
}

func TestExpandGlobPathsLiteralMetachars(t *testing.T) {
	dir := t.TempDir()
	literal := filepath.Join(dir, "file[1].go")
	writeFiles(t, literal)

	got := expandGlobPaths(literal)
	if len(got) != 1 || got[0] != literal {
		t.Fatalf("literal metachars = %v, want [%s]", got, literal)
	}

	if got := expandGlobPaths(filepath.Join(dir, "nope[1].go")); len(got) != 0 {
		t.Fatalf("missing literal metachars = %v, want empty", got)
	}
}

// writeUntilChange rewrites file until a ChangeSet arrives. macOS kqueue may
// report the first write after Add as a CHMOD-only event, which the watcher
// intentionally drops, so a single write is not a reliable probe there.
func writeUntilChange(t *testing.T, changes <-chan ChangeSet, file string, deadline time.Duration) ChangeSet {
	t.Helper()
	stop := time.Now().Add(deadline)
	for {
		writeFiles(t, file)

		wait := 200 * time.Millisecond
		if left := time.Until(stop); left < wait {
			wait = left
		}

		select {
		case change := <-changes:
			return change
		case <-time.After(wait):
		}

		if time.Now().After(stop) {
			t.Fatalf("no ChangeSet for %s within %v", file, deadline)
		}
	}
}

func assertNoChange(t *testing.T, changes <-chan ChangeSet, window time.Duration) {
	t.Helper()
	select {
	case change := <-changes:
		t.Fatalf("unexpected ChangeSet: %v", change.Files)
	case <-time.After(window):
	}
}

func TestWatchGlobOnlyWatchesMatches(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a_test.go")
	b := filepath.Join(dir, "b.go")
	writeFiles(t, a, b)

	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	w.Add(filepath.Join(dir, "*_test.go"))
	w.Watch(10 * time.Millisecond)
	defer w.Close()

	list := w.watcher.WatchList()
	if len(list) != 1 || list[0] != a {
		t.Fatalf("watch list = %v, want [%s]", list, a)
	}
}

func TestWatchGlobDeliversChanges(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a_test.go")
	b := filepath.Join(dir, "b.go")
	writeFiles(t, a, b)

	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	w.Add(filepath.Join(dir, "*_test.go"))
	changes := w.Watch(20 * time.Millisecond)
	defer w.Close()

	change := writeUntilChange(t, changes, a, 2*time.Second)
	if _, ok := change.Files[a]; !ok {
		t.Fatalf("ChangeSet files = %v, want %s", change.Files, a)
	}

	// b.go is not matched by the glob, so no watch covers it.
	writeFiles(t, b)
	assertNoChange(t, changes, 300*time.Millisecond)
}

func TestWatchLiteralBracketDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub[1]")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "a.go")
	writeFiles(t, file)

	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	w.Add(sub)
	changes := w.Watch(20 * time.Millisecond)
	defer w.Close()

	change := writeUntilChange(t, changes, file, 2*time.Second)
	if _, ok := change.Files[file]; !ok {
		t.Fatalf("ChangeSet files = %v, want %s", change.Files, file)
	}
}
