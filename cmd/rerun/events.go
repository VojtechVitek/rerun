package main

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// eventKind is what a target runner just did.
type eventKind int

const (
	evOutput     eventKind = iota // one line of the target's own output
	evChanged                     // a file-change batch restarted the target
	evStarted                     // a new generation is live
	evExited                      // a generation ended on its own
	evWatcherErr                  // the file watcher failed
	evRunnerErr                   // rerun could not start a generation
	evCleared                     // the screen clear the ad-hoc mode prints
)

// event is everything a target runner reports. The front-ends render it: the
// plain sink as prefixed text, the TUI as target rows and stream lines.
type event struct {
	kind   eventKind
	target string
	line   string
	count  int
	first  string
	status string
	code   int
	err    error
	at     time.Time
}

// sink is the front-end a target runner writes its events to.
type sink interface {
	emit(event)
}

// plainSink writes the prefixed plain text rerun printed before the TUI: the
// exact bytes non-TTY output and every existing test depend on.
type plainSink struct {
	out   io.Writer
	lines *sync.Mutex
}

func newPlainSink(out io.Writer) *plainSink {
	return &plainSink{out: out, lines: &sync.Mutex{}}
}

func (s *plainSink) emit(ev event) {
	s.lines.Lock()
	defer s.lines.Unlock()
	fmt.Fprint(s.out, s.render(ev))
}

// writer is the sink's direct writer, used by the ad-hoc path that passes the
// child's output through untouched.
func (s *plainSink) writer() io.Writer { return s.out }

func (s *plainSink) render(ev event) string {
	raw := ev.target == ""
	switch ev.kind {
	case evOutput:
		if raw {
			return ev.line + "\n"
		}
		return fmt.Sprintf("[%s] %s\n", ev.target, ev.line)

	case evChanged:
		plural := ""
		if ev.count > 1 {
			plural = "s"
		}
		if raw {
			return fmt.Sprintf("%s%s# %d file%s changed (e.g. %s)%s\n", clearTerminal, greenColor, ev.count, plural, ev.first, resetColor)
		}
		return fmt.Sprintf("[%s] %d file%s changed (e.g. %s)\n", ev.target, ev.count, plural, ev.first)

	case evExited:
		if raw {
			return fmt.Sprintf("rerun: exited (%s) — waiting for changes\n", ev.status)
		}
		return fmt.Sprintf("[%s] exited (%s) — waiting for changes\n", ev.target, ev.status)

	case evWatcherErr:
		if raw {
			return fmt.Sprintf("%v\n", ev.err)
		}
		return fmt.Sprintf("[%s] watcher error: %v\n", ev.target, ev.err)

	case evRunnerErr:
		return fmt.Sprintf("rerun: %s%v\n", s.label(ev.target), ev.err)

	case evCleared:
		if raw {
			return clearTerminal + "\n"
		}
	}

	return ""
}

// label is the target's identity in rerun's own messages.
func (s *plainSink) label(target string) string {
	if target == "" {
		return ""
	}
	return target + ": "
}
