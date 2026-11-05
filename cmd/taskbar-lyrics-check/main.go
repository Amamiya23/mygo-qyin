// taskbar-lyrics-check exercises the production taskbar controller with offline lyrics.
package main

import (
	"flag"
	"log"
	"time"

	"lilt/internal/app"
)

func main() {
	dir := flag.String("output", "build/taskbar-lyrics-check", "isolated preferences and native report directory")
	cover := flag.String("cover", "", "optional local image for offline cover verification")
	duration := flag.Duration("duration", 44*time.Second, "automatically exit after the native checks")
	flag.Parse()
	if err := app.RunTaskbarLyricsCheck(*dir, *duration, *cover); err != nil {
		log.Fatal(err)
	}
}
