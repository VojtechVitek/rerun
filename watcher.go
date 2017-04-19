package rerun

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	done    chan struct{}
	watch   []string
	ignore  []string
}

type ChangeSet struct {
	files  map[string]struct{}
	errors []error
}

func NewWatcher() (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher: watcher,
		done:    make(chan struct{}),
	}

	return w, nil
}

func (w *Watcher) Add(paths ...string) {
	w.watch = append(w.watch, paths...)
	for _, path := range paths {
		fmt.Printf("Add %v\n", path)
	}
}

func (w *Watcher) Ignore(paths ...string) {
	w.ignore = append(w.ignore, paths...)
	for _, path := range paths {
		fmt.Printf("Ignore %v\n", path)
	}
}

func (w *Watcher) Watch(delay time.Duration) chan ChangeSet {
	// resolve add + ignore paths
	for _, path := range w.watch { //s
		filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".git") {
				return filepath.SkipDir
			}

			w.watcher.Add(path)
			fmt.Printf("watch %v\n", path)
			return nil
		})
	}

	changes := make(chan ChangeSet, 1)

	go func() {
		for {
			change := ChangeSet{
				files: make(map[string]struct{}),
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

					timeout.Reset(delay)

					log.Printf("event: %v (%v)\n", event, time.Now()) //
					// if event.Op&fsnotify.Write == fsnotify.Write {
					// 	log.Println("modified file:", event.Name)
					// }
					change.files[event.Name] = struct{}{}

				case err := <-w.watcher.Errors:
					change.errors = append(change.errors, err)

				case <-timeout.C:
					changes <- change
					timeout.Stop()
					break loop
				}
			}
		}
	}()

	return changes
}

func (w *Watcher) Close() error {
	return w.watcher.Close()
}

func (c *ChangeSet) String() string {
	return fmt.Sprintf("ChangeSet: %v\nErrors: %v\n\n", c.files, c.errors)
}
