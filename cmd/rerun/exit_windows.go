//go:build windows

package main

import "os"

// signalName has no meaning on Windows; the exit code carries the result.
func signalName(*os.ProcessState) string { return "" }
