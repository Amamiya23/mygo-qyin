package app

import (
	"context"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"

	"lilt/internal/player"
)

// Only actual quit runs audio shutdown. Hiding the main window leaves it alone.
func (a *app) mainWindowClose(e *mygo.CloseEvent, hide, quit func()) {
	if a.quitting {
		return
	}
	e.PreventDefault()
	if !a.settings.QuitOnClose && a.canHideOnClose() {
		hide()
	} else {
		quit()
	}
}

var shutdownAudio = audio.Shutdown

func (a *app) finishAudio() {
	var fade time.Duration
	if a.settings.PlaybackFade {
		fade = player.FadeOutDuration
	}
	ctx, cancel := context.WithTimeout(context.Background(), fade+time.Second)
	defer cancel()
	if err := shutdownAudio(ctx, fade); err != nil {
		log.Printf("audio shutdown: %v", err)
	}
}
