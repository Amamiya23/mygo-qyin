//go:build windows || (darwin && !ios) || (linux && !android)

package app

// Album credits are currently designed for resizable native desktop windows.
const albumCreditsEnabled = true
