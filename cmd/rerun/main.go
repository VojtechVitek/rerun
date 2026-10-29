package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-cz/rerun"
)

const noGitignoreWarning = "rerun: warning: -no-gitignore is not implemented yet"

// warnNoGitignore reports the flag once; the gitignore engine lands later.
func warnNoGitignore(args Args, stderr io.Writer) {
	if args.NoGitignore {
		fmt.Fprintln(stderr, noGitignoreWarning)
	}
}

var version = "dev"

const usage = `rerun - re-run a command on file system changes

Usage:
  rerun                                run all rerun.yml targets in parallel
  rerun <target> [target...]           run specific rerun.yml targets
  rerun -- <cmd>                       ad-hoc run: watch ./ and run <cmd>
  rerun a b --except c d -- <cmd>      bare words = watchlist, --except = ignorelist
  rerun --on a b --except c d -- <cmd> --on is an explicit watchlist keyword

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
	}

	warnNoGitignore(args, os.Stderr)

	if len(args.Cmd) == 0 {
		os.Exit(runConfigMode(args))
	}

	runAdHoc(args)
}

// runConfigMode runs rerun.yml targets and returns the process exit code.
func runConfigMode(args Args) int {
	// Several bare words only name targets when the config lists them all.
	if len(args.Targets) > 0 {
		cfg, err := LoadConfig(".")
		if err != nil || !allKnown(cfg, args.Targets) {
			fmt.Fprintf(os.Stderr, "rerun: %v\n", noCmdHint)
			return 1
		}
		targets := lookupAll(cfg, args.Targets)
		applyOverrides(targets, args)
		return runTargets(targets, os.Stdout, nil)
	}

	cfg, err := LoadConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "rerun: %v\n", err)
		if args.Target == "" {
			fmt.Fprint(os.Stdout, usage)
		}
		return 1
	}

	if args.Target != "" {
		target, ok := cfg.Find(args.Target)
		if !ok {
			fmt.Fprintf(os.Stderr, "rerun: unknown target %q; available: %s\n", args.Target, strings.Join(cfg.Names(), ", "))
			return 1
		}
		targets := []Target{target}
		applyOverrides(targets, args)
		return runTargets(targets, os.Stdout, nil)
	}

	if len(cfg.Targets) == 0 {
		fmt.Fprintf(os.Stderr, "rerun: %s defines no targets\n", cfg.Path)
		return 1
	}
	targets := append([]Target(nil), cfg.Targets...)
	applyOverrides(targets, args)
	return runTargets(targets, os.Stdout, nil)
}

func allKnown(cfg *Config, names []string) bool {
	for _, name := range names {
		if _, ok := cfg.Find(name); !ok {
			return false
		}
	}
	return true
}

func lookupAll(cfg *Config, names []string) []Target {
	targets := make([]Target, 0, len(names))
	for _, name := range names {
		target, _ := cfg.Find(name)
		targets = append(targets, target)
	}
	return targets
}

// applyOverrides lets CLI flags win over the target's own timeouts.
func applyOverrides(targets []Target, args Args) {
	for i := range targets {
		if args.Delay > 0 {
			targets[i].Delay = args.Delay
		}
		if args.Cooldown > 0 {
			targets[i].Cooldown = args.Cooldown
		}
		if args.Grace > 0 {
			targets[i].Grace = args.Grace
		}
	}
}

// runAdHoc is the single-command path: watch, restart on change.
func runAdHoc(args Args) {
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

	grace := rerun.DefaultGrace
	if args.Grace > 0 {
		grace = args.Grace
	}

	cooldown := args.Cooldown
	var lastStart time.Time

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	cmd, err := rerun.StartCommand(args.Cmd, grace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rerun: %v\n", err)
		os.Exit(1)
	}
	lastStart = time.Now()

	go func() {
		<-sig

		go func() {
			<-sig // Second signal: skip the grace period.
			_ = cmd.KillForce()
			os.Exit(130)
		}()

		if err := cmd.Kill(); err != nil {
			fmt.Printf("%v\n", err)
		}

		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-done:
					return
				case <-time.After(1 * time.Second):
					fmt.Printf("\033cWaiting on %v\n", cmd.PID())
				}
			}
		}()
		_ = cmd.WaitExit(context.Background())
		close(done)

		os.Exit(0)
	}()

	fmt.Printf("%s\n", clearTerminal) // comment out for debugging
	for changeSet := range watcher.Watch(delay) {
		// A command that exited by itself was already reaped by its reaper.
		if !cmd.Exited() {
			if err := cmd.Kill(); err != nil {
				fmt.Printf("%v\n", err)
			}
			_ = cmd.WaitExit(context.Background())
		}
		if changeSet.Error != nil {
			fmt.Printf("%v\n", changeSet.Error)
		}

		waitCooldown(cooldown, lastStart)

		plural := ""
		if len(changeSet.Files) > 1 {
			plural = "s"
		}
		fmt.Printf("%s%s# %v file%v changed (e.g. %v)%s\n", clearTerminal, greenColor, len(changeSet.Files), plural, changeSet.FirstFile, resetColor)

		if err := cmd.Start(); err != nil {
			fmt.Printf("%v\n", err)
		}
		lastStart = time.Now()
	}

	select {}
}

// waitCooldown sleeps out the remainder of the cooldown window since the last
// start; zero disables it.
func waitCooldown(cooldown time.Duration, lastStart time.Time) {
	if cooldown <= 0 {
		return
	}
	if remaining := cooldown - time.Since(lastStart); remaining > 0 {
		time.Sleep(remaining)
	}
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
