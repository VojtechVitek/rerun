package rerun

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher     *fsnotify.Watcher
	watch       map[string]struct{}
	ignore      map[string]struct{}
	ignoreDirs  map[string]struct{}
	ignoreFiles map[string]struct{}
	done        chan struct{}
	watchOnce   sync.Once
	closeOnce   sync.Once
}

type ChangeSet struct {
	FirstFile string
	Files     map[string]struct{}
	Error     error
}

func NewWatcher() (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher:     watcher,
		watch:       make(map[string]struct{}),
		ignore:      make(map[string]struct{}),
		ignoreDirs:  make(map[string]struct{}),
		ignoreFiles: make(map[string]struct{}),
		done:        make(chan struct{}),
	}

	return w, nil
}

// Add registers a path to watch. Configure watches before calling Watch.
func (w *Watcher) Add(paths ...string) {
	for _, path := range paths {
		w.watch[path] = struct{}{}
	}
}

// Ignore registers a path or glob to skip. Configure ignores before Watch.
func (w *Watcher) Ignore(paths ...string) {
	for _, path := range paths {
		w.ignore[path] = struct{}{}
	}
}

// Watch starts the event goroutine and returns its ChangeSet channel. It is
// single-use: a second call returns a nil channel.
func (w *Watcher) Watch(delay time.Duration) <-chan ChangeSet {
	var changes chan ChangeSet

	w.watchOnce.Do(func() {
		// resolve add + ignore paths
		for path := range w.watch {
			for _, root := range expandGlobPaths(path) {
				filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return nil
					}

					if info.IsDir() {
						// Dirs
						if strings.HasSuffix(path, ".git") {
							return filepath.SkipDir
						}

						if _, ok := w.ignore[path]; ok {
							w.ignoreDirs[path] = struct{}{}
							return filepath.SkipDir
						}

						w.watcher.Add(path)
						return nil

					} else {

						// Files
						// Check if file path matches any ignore pattern
						for ignorePath := range w.ignore {
							if path == ignorePath || strings.HasPrefix(path, ignorePath+string(os.PathSeparator)) {
								// Check for exact match or if the file is in an ignored directory
								w.ignoreFiles[path] = struct{}{}
								return nil
							} else if strings.Contains(ignorePath, "*") {
								// Check for glob pattern match of filename
								if matchGlobPattern(ignorePath, path) {
									w.ignoreFiles[path] = struct{}{}
									return nil
								}
							}
						}

						// Watch single-file targets directly; fsnotify only watches dirs otherwise.
						if path == root {
							w.watcher.Add(path)
						}

						return nil
					}
				})
			}
		}

		changes = make(chan ChangeSet, 1)

		go func() {
			defer close(changes)

			// deliver hands the change to the consumer unless Close won the race.
			deliver := func(change ChangeSet) bool {
				select {
				case <-w.done:
					return false
				default:
				}
				select {
				case changes <- change:
					return true
				case <-w.done:
					return false
				}
			}

			for {
				change := ChangeSet{
					Files: make(map[string]struct{}),
				}

				// A nil timeoutC blocks forever, so no timer is needed before
				// the first event; the real timer is created on that event.
				var timeoutC <-chan time.Time
				var timer *time.Timer
				stopTimer := func() {
					if timer != nil {
						timer.Stop()
					}
				}

			deliverLoop:
				for {
					select {
					case event, ok := <-w.watcher.Events:
						if !ok {
							stopTimer()
							return
						}

						// Ignore CHMOD.
						if event.Op&fsnotify.Chmod == fsnotify.Chmod {
							continue
						}

						// NOTE: ignoreDirs is already ignored because it's never added to the watcher.
						if _, ok := w.ignoreFiles[event.Name]; ok {
							continue
						}

						if timeoutC == nil {
							timer = time.NewTimer(delay)
							timeoutC = timer.C
						} else {
							timer.Reset(delay)
						}

						if len(change.Files) == 0 {
							change.FirstFile = event.Name
						}
						change.Files[event.Name] = struct{}{}

					case err, ok := <-w.watcher.Errors:
						if !ok {
							stopTimer()
							return
						}
						change.Error = err
						if !deliver(change) {
							stopTimer()
							return
						}
						stopTimer()
						break deliverLoop

					case <-timeoutC:
						if !deliver(change) {
							stopTimer()
							return
						}
						stopTimer()
						break deliverLoop

					case <-w.done:
						stopTimer()
						return
					}
				}
			}
		}()
	})

	return changes
}

// Close stops the watcher. It is safe to call more than once.
func (w *Watcher) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.done)
		err = w.watcher.Close()
	})
	return err
}

func (c *ChangeSet) String() string {
	files := make([]string, 0, len(c.Files))
	for file := range c.Files {
		files = append(files, file)
	}
	sort.Strings(files)
	return strings.Join(files, "\n")
}

// expandGlobPaths expands a watch entry containing glob metachars into existing paths.
func expandGlobPaths(pattern string) []string {
	if !strings.ContainsAny(pattern, "*?[") {
		return []string{pattern}
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	if len(matches) == 0 {
		// A literal path can contain metachars (e.g. a dir named "foo[1]");
		// the glob finds nothing, but the path itself may exist.
		if _, err := os.Stat(pattern); err != nil {
			return nil
		}
		return []string{pattern}
	}

	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		if _, err := os.Stat(match); err != nil {
			continue
		}
		paths = append(paths, match)
	}

	return paths
}

// matchGlobPattern reports whether path matches pattern. The pattern is
// relative, so it matches any suffix of the path: `dir/*.md` matches
// `dir/a.md` and `/abs/dir/a.md`, but not `dir/sub/a.md`. `**` spans segments.
func matchGlobPattern(pattern, path string) bool {
	patternSegs := strings.Split(filepath.ToSlash(pattern), "/")
	pathSegs := strings.Split(filepath.ToSlash(path), "/")

	for i := 0; i <= len(pathSegs); i++ {
		if matchSegments(patternSegs, pathSegs[i:]) {
			return true
		}
	}
	return false
}

// matchSegments matches path segments against pattern segments, with "**"
// absorbing any number of segments.
func matchSegments(patternSegs, pathSegs []string) bool {
	for len(patternSegs) > 0 {
		if patternSegs[0] == "**" {
			if matchSegments(patternSegs[1:], pathSegs) {
				return true
			}
			if len(pathSegs) == 0 {
				return false
			}
			pathSegs = pathSegs[1:]
			continue
		}
		if len(pathSegs) == 0 {
			return false
		}
		matched, err := filepath.Match(patternSegs[0], pathSegs[0])
		if err != nil || !matched {
			return false
		}
		patternSegs, pathSegs = patternSegs[1:], pathSegs[1:]
	}
	return len(pathSegs) == 0
}
