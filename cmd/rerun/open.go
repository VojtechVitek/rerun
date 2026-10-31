package main

import (
	"os/exec"
	"runtime"
)

// openURL opens a url in the platform's browser. Tests replace it.
var openURL = openInBrowser

// openInBrowser hands a url to the platform's browser. The wait is reaped in
// the background so the TUI never blocks on the browser.
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// start is a cmd.exe builtin; the empty argument is the window title.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
