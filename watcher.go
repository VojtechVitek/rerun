package rerun

import (
	"fmt"
	"log"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	done    chan struct{}
	watch   []string
	ignore  []string
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

func (w *Watcher) Watch() error {
	// resolve add + ignore paths
	w.watcher.Add("./")

	for {
		select {
		case event := <-w.watcher.Events:
			log.Println("event:", event)
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Println("modified file:", event.Name)
			}
		case err := <-w.watcher.Errors:
			return err
		}
	}
}

func (w *Watcher) Close() error {
	return w.watcher.Close()
}
