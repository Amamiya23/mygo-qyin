//go:build !windows

package main

import "log"

func main() { log.Fatal("taskbar-lyrics-probe requires Windows") }
