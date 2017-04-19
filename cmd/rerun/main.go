package main

import (
	"fmt"
	"log"

	"os"

	"time"

	"github.com/VojtechVitek/rerun"
)

type argType int

const (
	argNone argType = iota
	argWatch
	argIgnore
	argRun
)

func main() {
	watcher, err := rerun.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	args := os.Args[1:]
	mode := argNone

	// Parse command line arguments.
	// -watch dirs ...
	// -ignore dirs ...
	// -run command ...
loop:
	for i, arg := range args {
		switch mode {
		case argNone, argWatch, argIgnore:
			switch arg {
			case "-watch":
				mode = argWatch
				continue
			case "-ignore":
				mode = argIgnore
				continue
			case "-run":
				mode = argRun
				continue
			}
		}

		switch mode {
		case argWatch:
			watcher.Add(arg)
		case argIgnore:
			watcher.Ignore(arg)
		case argRun:
			args = args[i:]
			break loop
		default:
			break loop
		}
	}

	if mode == argNone {
		log.Fatal("TODO: interactive mode")
	}

	fmt.Printf("\033c")
	cmd, err := rerun.Run(args...)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}

	for change := range watcher.Watch(200 * time.Millisecond) {
		fmt.Printf("\033c")
		_ = change
		//fmt.Printf("\033c%v\n", change)
		if err := cmd.Restart(); err != nil {
			fmt.Printf("ERROR: %v\n", err)
		}
	}

	watcher.Close()
}
