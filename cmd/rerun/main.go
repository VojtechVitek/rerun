package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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
	defer watcher.Close()

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

	cmd, err := rerun.StartCommand(args...)
	if err != nil {
		log.Fatal(err)
	}
	defer cmd.Kill()
	fmt.Printf("\033c%v\n", cmd)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig
		cmd.Kill()
		watcher.Close()
		os.Exit(1)
	}()

	for changeSet := range watcher.Watch(200 * time.Millisecond) {
		cmd.Kill()

		if changeSet.Error != nil {
			log.Fatal(err)
		}

		plural := ""
		if len(changeSet.Files) > 1 {
			plural = "s"
		}
		fmt.Printf("\033c\033[32m# %v file%v changed.\033[0m\n%v\n", len(changeSet.Files), plural, cmd)

		if err := cmd.Start(); err != nil {
			fmt.Printf("ERROR: %v\n", err)
		}
	}
}
