package rerun

import (
	"fmt"
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	done    chan struct{}
	watch   []string
	ignore  []string
}

type Change struct {
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

func (w *Watcher) Watch(delay time.Duration) chan Change {
	// resolve add + ignore paths
	w.watcher.Add("./")

	changes := make(chan Change, 1)

	go func() {
		for {
			change := Change{
				files: make(map[string]struct{}),
			}

			timeout := time.NewTimer(1<<63 - 1) // max duration
			timeout.Stop()
			first := true

			for {
				select {
				case event := <-w.watcher.Events:
					if first {
						first = false
						timeout = time.NewTimer(delay)
					}

					log.Println("event:", event)
					if event.Op&fsnotify.Write == fsnotify.Write {
						log.Println("modified file:", event.Name)
					}
					change.files[event.Name] = struct{}{}

				case err := <-w.watcher.Errors:
					if first {
						first = false
						timeout = time.NewTimer(delay)
					}

					change.errors = append(change.errors, err)

				case <-timeout.C:
					changes <- change
					break
				}
			}
		}
	}()

	return changes
}

func (w *Watcher) Close() error {
	return w.watcher.Close()
}

func (c *Change) String() string {
	return fmt.Sprintf("%v\nerrors: %v\n\n", c.files, c.errors)
}
