//go:build !(windows || (darwin && !ios) || (linux && !android))

package app

// Enable after the album footer has been adapted for this platform.
const albumCreditsEnabled = false
