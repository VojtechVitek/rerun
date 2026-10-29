//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// signalNames renames syscall.Signal's descriptions ("terminated") to the
// POSIX names a developer recognises.
var signalNames = map[syscall.Signal]string{
	syscall.SIGHUP:  "SIGHUP",
	syscall.SIGINT:  "SIGINT",
	syscall.SIGQUIT: "SIGQUIT",
	syscall.SIGILL:  "SIGILL",
	syscall.SIGTRAP: "SIGTRAP",
	syscall.SIGABRT: "SIGABRT",
	syscall.SIGBUS:  "SIGBUS",
	syscall.SIGFPE:  "SIGFPE",
	syscall.SIGKILL: "SIGKILL",
	syscall.SIGSEGV: "SIGSEGV",
	syscall.SIGPIPE: "SIGPIPE",
	syscall.SIGALRM: "SIGALRM",
	syscall.SIGTERM: "SIGTERM",
	syscall.SIGUSR1: "SIGUSR1",
	syscall.SIGUSR2: "SIGUSR2",
	syscall.SIGXCPU: "SIGXCPU",
	syscall.SIGXFSZ: "SIGXFSZ",
	syscall.SIGSYS:  "SIGSYS",
}

// signalName returns the signal that killed ps, or "" for a normal exit.
func signalName(ps *os.ProcessState) string {
	if ps == nil {
		return ""
	}
	ws, ok := ps.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return ""
	}

	sig := ws.Signal()
	if name, ok := signalNames[sig]; ok {
		return name
	}
	return fmt.Sprintf("SIG%d", int(sig))
}
