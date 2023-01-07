package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goware/rerun"
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
		log.Fatal("Please see usage at https://github.com/goware/rerun")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	cmd, err := rerun.StartCommand(args...)
	if err != nil {
		log.Fatal(err)
	}
	defer cmd.Kill()

	go func() {
		<-sig

		if err := cmd.Kill(); err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}

		go func() {
			<-sig // Double kill, exit now.
			os.Exit(1)
		}()

		done := make(chan struct{}, 0)
		go func() {
			for {
				select {
				case <-done:
					return
				case <-time.After(1 * time.Second):
					fmt.Printf("\033cWaiting on PID %v\n", cmd.PID())
				}
			}
		}()
		cmd.Wait()
		close(done)

		os.Exit(1)
	}()

	fmt.Printf("%s\n", clear)
	for changeSet := range watcher.Watch(200 * time.Millisecond) {
		if err := cmd.Kill(); err != nil {
			fmt.Printf("%v\n", err)
		}
		cmd.Wait()
		if changeSet.Error != nil {
			fmt.Printf("%v\n", err)
		}

		plural := ""
		if len(changeSet.Files) > 1 {
			plural = "s"
		}
		fmt.Printf("%s%s# %v file%v changed (ie. %v)%s\n", clear, greenColor, len(changeSet.Files), plural, changeSet.FirstFile, resetColor)

		if err := cmd.Start(); err != nil {
			fmt.Printf("%v\n", err)
		}
	}

	select {}
}

const (
	clear      = "\033c"
	greenColor = "\033[32m"
	resetColor = "\033[0m"
)
