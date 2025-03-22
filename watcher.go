package rerun

import (
	"os"
	"path/filepath"
	"strings"
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

func (w *Watcher) Add(paths ...string) {
	for _, path := range paths {
		w.watch[path] = struct{}{}
		// fmt.Printf("Add %v\n", path)
	}
}

func (w *Watcher) Ignore(paths ...string) {
	for _, path := range paths {
		w.ignore[path] = struct{}{}
		// fmt.Printf("Ignore %v\n", path)
	}
}

func (w *Watcher) Watch(delay time.Duration) <-chan ChangeSet {
	// fmt.Println()

	// resolve add + ignore paths
	for path := range w.watch {
		filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if info == nil {
				return nil
			}

			if info.IsDir() {
				// Dirs
				if strings.HasSuffix(path, ".git") {
					//fmt.Printf("skip %v\n", path)
					return filepath.SkipDir
				}

				if _, ok := w.ignore[path]; ok {
					// fmt.Printf("skip %v\n", path)
					w.ignoreDirs[path] = struct{}{}
					return filepath.SkipDir
				}

				w.watcher.Add(path)
				// fmt.Printf("watch %v\n", path)
				return nil

			} else {

				// Files
				// Check if file path matches any ignore pattern
				for ignorePath := range w.ignore {
					if path == ignorePath || strings.HasPrefix(path, ignorePath+string(os.PathSeparator)) {
						// Check for exact match or if the file is in an ignored directory
						// fmt.Printf("ignore file %v\n", path)
						w.ignoreFiles[path] = struct{}{}
						return nil
					} else if strings.Contains(ignorePath, "*") {
						// Check for glob pattern match of filename
						if matchGlobPattern(ignorePath, path) {
							// fmt.Printf("ignore file (glob match) %v\n", path)
							w.ignoreFiles[path] = struct{}{}
							return nil
						}
					}
				}

				return nil
			}
		})
	}
	//	fmt.Println()

	changes := make(chan ChangeSet, 1)

	go func() {
		for {
			change := ChangeSet{
				Files: make(map[string]struct{}),
			}

			timeout := time.NewTimer(1<<63 - 1) // max duration
			timeout.Stop()

		loop:
			for {
				select {
				case event := <-w.watcher.Events:
					// Ignore CHMOD.
					if event.Op&fsnotify.Chmod == fsnotify.Chmod {
						continue
					}

					// Ignore change if it's in the ignoreFiles list.
					// NOTE: ignoreDirs is already ignored because its never added to the watcher.
					if _, ok := w.ignoreFiles[event.Name]; ok {
						continue
					}

					// fmt.Printf("event: %v (%v)\n", event, time.Now()) //

					timeout.Reset(delay)

					//fmt.Printf("event: %v (%v)\n", event, time.Now()) //
					// if event.Op&fsnotify.Write == fsnotify.Write {
					// 	log.Println("modified file:", event.Name)
					// }
					if len(change.Files) == 0 {
						change.FirstFile = event.Name
					}
					change.Files[event.Name] = struct{}{}

				case err := <-w.watcher.Errors:
					change.Error = err
					changes <- change
					timeout.Stop()
					break loop

				case <-timeout.C:
					changes <- change
					timeout.Stop()
					break loop

				case <-w.done:
					close(changes)
					timeout.Stop()
					return
				}
			}
		}
	}()

	return changes
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.watcher.Close()
}

func (c *ChangeSet) String() string {
	str := ""
	for file, _ := range c.Files {
		str += "\n" + file
	}
	return str
}

// matchGlobPattern checks if a path matches a glob pattern
func matchGlobPattern(pattern, path string) bool {
	// Convert the glob pattern to a filepath.Match compatible pattern
	matched, err := filepath.Match(pattern, filepath.Base(path))
	if err != nil {
		// fmt.Printf("invalid pattern %q: %v\n", pattern, err)
		return false
	}

	// For patterns like "*.md", only match the filename
	if strings.HasPrefix(pattern, "*") {
		return matched
	}

	// For patterns with directory components like "dir/*.md"
	// Check if the directory part matches
	patternDir := filepath.Dir(pattern)
	if patternDir != "." {
		pathDir := filepath.Dir(path)
		if !strings.HasPrefix(pathDir, patternDir) {
			return false
		}
	}

	return matched
}
