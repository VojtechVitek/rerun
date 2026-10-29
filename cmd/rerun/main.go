package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-cz/rerun"
)

var version = "dev"

const usage = `rerun - re-run a command on file system changes

Usage:
  rerun -- <cmd>                        ad-hoc run: watch ./ and run <cmd>
  rerun a b --except c d -- <cmd>       bare words = watchlist, --except = ignorelist
  rerun --on a b --except c d -- <cmd>  --on is an explicit watchlist keyword
  rerun <target>                        run a rerun.yml target (not supported yet)

Lists are also read from stdin when it is not a TTY:
  find . -name '*.go' | rerun -- go test

Everything after -- is the command, passed through verbatim.

Flags:
  -delay <dur>      batch window before restarting a command (default 200ms)
  -cooldown <dur>   minimum time between restarts (default 0s)
  -grace <dur>      time between SIGTERM and SIGKILL (default 5s)
  -no-gitignore     do not respect .gitignore
  -h, -help         show this help
  -version          show version

Deprecated: -watch, -ignore and -run (use --on, --except and --).
`

func main() {
	args, err := Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rerun: %v\n", err)
		os.Exit(1)
	}

	switch {
	case args.Help:
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	case args.Version:
		fmt.Fprintf(os.Stdout, "rerun %s\n", version)
		os.Exit(0)
	case args.Target != "":
		fmt.Fprintln(os.Stderr, "rerun: rerun.yml targets are not supported yet — use: rerun -- <cmd>")
		os.Exit(1)
	}

	watcher, err := rerun.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "rerun: %v\n", err)
		os.Exit(1)
	}

	watch := args.Watch
	if len(watch) == 0 {
		watch = []string{"./"}
	}
	watcher.Add(watch...)
	if len(args.Except) > 0 {
		watcher.Ignore(args.Except...)
	}

	delay := 200 * time.Millisecond
	if args.Delay > 0 {
		delay = args.Delay
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	cmd, err := rerun.StartCommand(args.Cmd...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rerun: %v\n", err)
		os.Exit(1)
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

	fmt.Printf("%s\n", clearTerminal) // comment out for debugging
	for changeSet := range watcher.Watch(delay) {
		if err := cmd.Kill(); err != nil {
			fmt.Printf("%v\n", err)
		}
		cmd.Wait()
		if changeSet.Error != nil {
			fmt.Printf("%v\n", changeSet.Error)
		}

		plural := ""
		if len(changeSet.Files) > 1 {
			plural = "s"
		}
		fmt.Printf("%s%s# %v file%v changed (e.g. %v)%s\n", clearTerminal, greenColor, len(changeSet.Files), plural, changeSet.FirstFile, resetColor)

		if err := cmd.Start(); err != nil {
			fmt.Printf("%v\n", err)
		}
	}

	select {}
}

const (
	// clearScrollbackXterm clears the scrollback buffer in terminals
	// supporting the standard xterm sequence ESC[3J.
	clearScrollbackXterm = "\033[3J"

	// clearScreen moves the cursor to the home position (ESC[H)
	// and clears the visible screen (ESC[2J).
	clearScreen = "\033[H\033[2J"

	// clearScrollbackITerm is iTerm2's proprietary escape code that clears
	// the entire scrollback buffer. Required because iTerm2 ignores ESC[3J.
	clearScrollbackITerm = "\033]1337;ClearScrollback\a"

	// clearTerminal combines all sequences above to thoroughly clear both
	// the visible screen and full scrollback history across major terminals.
	clearTerminal = clearScrollbackXterm + clearScreen + clearScrollbackITerm

	greenColor = "\033[32m"
	resetColor = "\033[0m"
)
