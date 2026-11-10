package main

import (
	"flag"
	"lilt/internal/app"
	"log"
	"time"
)

func main() {
	dir := flag.String("output", "build/desktop-lyrics-probe", "isolated preferences and screenshot directory")
	duration := flag.Duration("duration", 2*time.Minute, "automatically exit after this duration; 0 waits for Exit")
	check := flag.Bool("check", false, "exercise sizing, visibility, locking, reopening and bottom-edge placement")
	flag.Parse()
	if err := app.RunDesktopLyricsProbe(*dir, *duration, *check); err != nil {
		log.Fatal(err)
	}
}
