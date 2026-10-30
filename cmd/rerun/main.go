package main

import (
	"fmt"
	"io"
	"os"
	"strings"

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

	os.Exit(runAdHoc(args, os.Stdout, nil))
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

// adHocTarget is the synthetic nameless target for `rerun -- <cmd>`. An empty
// Name is what selects the raw passthrough output mode.
func adHocTarget(args Args) Target {
	targets := []Target{{
		Cmd:    rerun.ShellScript(args.Cmd),
		On:     args.Watch,
		Except: args.Except,
		Delay:  defaultDelay,
		Grace:  rerun.DefaultGrace,
	}}
	applyOverrides(targets, args)
	return targets[0]
}

// runAdHoc is the single-command path: watch, restart on change. A nil
// signals channel installs the default handlers.
func runAdHoc(args Args, out io.Writer, signals <-chan os.Signal) int {
	return runTargets([]Target{adHocTarget(args)}, out, signals)
}
