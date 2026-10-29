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
				r.printf("rerun: %s: %v\n", r.target.Name, err)
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

	on := r.target.On
	if len(on) == 0 {
		on = []string{"./"}
	}
	watcher.Add(on...)
	if len(r.target.Except) > 0 {
		watcher.Ignore(r.target.Except...)
	}

	changes := watcher.Watch(r.target.Delay)

	if err := r.start(); err != nil {
		return err
	}

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
		r.printf("[%s] watcher error: %v\n", r.target.Name, changeSet.Error)
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
	r.printf("[%s] %d file%s changed (e.g. %s)\n", r.target.Name, len(changeSet.Files), plural, changeSet.FirstFile)

	if err := r.start(); err != nil {
		r.printf("rerun: %s: %v\n", r.target.Name, err)
	}
}

// start spawns a new generation with its own output pipe.
func (r *targetRunner) start() error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}

	cmd, err := rerun.StartCommand([]string{r.target.Cmd}, r.target.Grace,
		rerun.WithStdout(pw), rerun.WithStderr(pw))
	if err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return err
	}

	go r.prefix(pr)
	exited := r.watchGeneration(cmd, pw)

	r.mu.Lock()
	r.cmd = cmd
	r.lastStart = time.Now()
	r.mu.Unlock()

	r.exited = exited
	return nil
}

// watchGeneration closes the pipe writer once the generation is reaped, which
// unblocks the prefixer's read side.
func (r *targetRunner) watchGeneration(cmd rerun.Command, pw *os.File) <-chan struct{} {
	exited := make(chan struct{})
	go func() {
		_ = cmd.WaitExit(context.Background())
		_ = pw.Close()
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

	r.printf("[%s] exited (%s) — waiting for changes\n", r.target.Name, exitDescription(cmd))
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
