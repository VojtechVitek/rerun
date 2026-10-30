package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang-cz/rerun"
)

// runTargets runs every target in parallel until a shutdown signal arrives.
//
// It returns 0 after a graceful shutdown and 130 when a second signal forces
// the kill. A nil signals channel installs the default handlers.
func runTargets(targets []Target, out io.Writer, signals <-chan os.Signal) int {
	if out == nil {
		out = os.Stdout
	}
	if signals == nil {
		ch := make(chan os.Signal, 4)
		signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		defer signal.Stop(ch)
		signals = ch
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines := &sync.Mutex{}
	runners := make([]*targetRunner, 0, len(targets))
	for _, target := range targets {
		runners = append(runners, newTargetRunner(target, out, lines))
	}

	var failed int32
	var wg sync.WaitGroup
	for _, r := range runners {
		wg.Add(1)
		go func(r *targetRunner) {
			defer wg.Done()
			if err := r.run(ctx); err != nil {
				atomic.StoreInt32(&failed, 1)
				r.printf("rerun: %s%v\n", r.label(), err)
			}
		}(r)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-signals:
	case <-done:
		if atomic.LoadInt32(&failed) != 0 {
			return 1
		}
		return 0
	}

	// First signal: graceful kill. A second one skips the grace period.
	for _, r := range runners {
		r.markStopping()
	}

	var forced int32
	stop := make(chan struct{})
	go func() {
		select {
		case <-signals:
			atomic.StoreInt32(&forced, 1)
			for _, r := range runners {
				r.force()
			}
		case <-stop:
		}
	}()

	var killWG sync.WaitGroup
	for _, r := range runners {
		killWG.Add(1)
		go func(r *targetRunner) {
			defer killWG.Done()
			r.stop()
		}(r)
	}
	killWG.Wait()

	cancel()
	<-done
	close(stop)

	if atomic.LoadInt32(&forced) != 0 {
		return 130
	}
	return 0
}

// targetRunner owns one target's watcher and its current child process.
type targetRunner struct {
	target Target
	out    io.Writer
	lines  *sync.Mutex

	mu        sync.Mutex
	cmd       rerun.Command
	lastStart time.Time
	stopping  int32

	// exited is closed when the current generation is reaped. Only the run
	// goroutine reads it, so no lock is needed.
	exited <-chan struct{}
}

func newTargetRunner(target Target, out io.Writer, lines *sync.Mutex) *targetRunner {
	return &targetRunner{target: target, out: out, lines: lines}
}

func (r *targetRunner) run(ctx context.Context) error {
	watcher, err := rerun.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Close the watcher on shutdown: the initial walk below can take seconds on
	// a big tree and would otherwise hold the whole run back.
	closeWatcher := make(chan struct{})
	defer close(closeWatcher)
	go func() {
		select {
		case <-ctx.Done():
			_ = watcher.Close()
		case <-closeWatcher:
		}
	}()

	on := r.target.On
	if len(on) == 0 {
		on = []string{"./"}
	}
	watcher.Add(on...)
	if len(r.target.Except) > 0 {
		watcher.Ignore(r.target.Except...)
	}

	// Start before the watch walk: the walk can take seconds on a big tree,
	// and the command's output must not wait for it.
	if err := r.start(); err != nil {
		return err
	}
	if r.raw() {
		r.printf("%s\n", clearTerminal) // comment out for debugging
	}

	changes := watcher.Watch(r.target.Delay)

	for {
		select {
		case <-ctx.Done():
			r.killCurrent(context.Background())
			return nil

		case changeSet, ok := <-changes:
			if !ok {
				r.killCurrent(context.Background())
				return nil
			}
			r.handleChange(ctx, changeSet)

		case <-r.exited:
			r.reportExit()
			r.exited = nil
		}
	}
}

func (r *targetRunner) handleChange(ctx context.Context, changeSet rerun.ChangeSet) {
	if changeSet.Error != nil {
		if r.raw() {
			r.printf("%v\n", changeSet.Error)
		} else {
			r.printf("[%s] watcher error: %v\n", r.target.Name, changeSet.Error)
		}
		return
	}

	r.killCurrent(ctx)

	r.waitCooldown(ctx)
	if ctx.Err() != nil {
		return
	}

	plural := ""
	if len(changeSet.Files) > 1 {
		plural = "s"
	}
	if r.raw() {
		r.printf("%s%s# %d file%s changed (e.g. %s)%s\n", clearTerminal, greenColor,
			len(changeSet.Files), plural, changeSet.FirstFile, resetColor)
	} else {
		r.printf("[%s] %d file%s changed (e.g. %s)\n", r.target.Name, len(changeSet.Files), plural, changeSet.FirstFile)
	}

	if err := r.start(); err != nil {
		r.printf("rerun: %s%v\n", r.label(), err)
	}
}

// start spawns a new generation.
func (r *targetRunner) start() error {
	var (
		cmd rerun.Command
		pw  *os.File
		err error
	)
	if r.raw() {
		// The ad-hoc command is passed through raw: no pipe, no prefixing.
		cmd, err = rerun.StartCommand([]string{r.target.Cmd}, r.target.Grace, rerun.WithStdout(r.out))
	} else {
		cmd, pw, err = r.startPiped()
	}
	if err != nil {
		return err
	}

	exited := r.watchGeneration(cmd, pw)

	r.mu.Lock()
	r.cmd = cmd
	r.lastStart = time.Now()
	r.mu.Unlock()

	r.exited = exited
	return nil
}

// startPiped spawns a generation whose output is prefixed line by line.
func (r *targetRunner) startPiped() (rerun.Command, *os.File, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	cmd, err := rerun.StartCommand([]string{r.target.Cmd}, r.target.Grace,
		rerun.WithStdout(pw), rerun.WithStderr(pw))
	if err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return nil, nil, err
	}

	go r.prefix(pr)
	return cmd, pw, nil
}

// watchGeneration closes the pipe writer once the generation is reaped, which
// unblocks the prefixer's read side.
func (r *targetRunner) watchGeneration(cmd rerun.Command, pw *os.File) <-chan struct{} {
	exited := make(chan struct{})
	go func() {
		_ = cmd.WaitExit(context.Background())
		if pw != nil {
			_ = pw.Close()
		}
		close(exited)
	}()
	return exited
}

func (r *targetRunner) prefix(pr *os.File) {
	defer pr.Close()

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		r.printf("[%s] %s\n", r.target.Name, scanner.Text())
	}
}

func (r *targetRunner) reportExit() {
	if atomic.LoadInt32(&r.stopping) != 0 {
		return
	}

	cmd := r.currentCmd()
	if cmd == nil {
		return
	}

	code, err := cmd.ExitState()
	status := describeExit(code, err)
	if r.raw() {
		r.printf("rerun: exited (%s) — waiting for changes\n", status)
		return
	}
	r.printf("[%s] exited (%s) — waiting for changes\n", r.target.Name, status)
}

// raw reports whether the target passes output through unprefixed. The ad-hoc
// command is the only nameless target.
func (r *targetRunner) raw() bool { return r.target.Name == "" }

// label is the target's identity in rerun's own messages.
func (r *targetRunner) label() string {
	if r.raw() {
		return ""
	}
	return r.target.Name + ": "
}

// killCurrent stops the current generation, waiting the grace period.
func (r *targetRunner) killCurrent(ctx context.Context) {
	cmd := r.currentCmd()
	if cmd == nil || cmd.Exited() {
		return
	}
	_ = cmd.Kill()
	_ = cmd.WaitExit(ctx)
}

// waitCooldown holds a restart back until the cooldown since the last start.
func (r *targetRunner) waitCooldown(ctx context.Context) {
	if r.target.Cooldown <= 0 {
		return
	}

	r.mu.Lock()
	last := r.lastStart
	r.mu.Unlock()

	remaining := r.target.Cooldown - time.Since(last)
	if remaining <= 0 {
		return
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (r *targetRunner) markStopping() {
	atomic.StoreInt32(&r.stopping, 1)
}

func (r *targetRunner) stop() {
	cmd := r.currentCmd()
	if cmd == nil {
		return
	}
	_ = cmd.Kill()
}

func (r *targetRunner) force() {
	cmd := r.currentCmd()
	if cmd == nil {
		return
	}
	_ = cmd.KillForce()
}

func (r *targetRunner) currentCmd() rerun.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd
}

func (r *targetRunner) printf(format string, args ...any) {
	r.lines.Lock()
	defer r.lines.Unlock()
	fmt.Fprintf(r.out, format, args...)
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
