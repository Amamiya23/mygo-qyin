package app

import (
	"context"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
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

func finishAudio() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := audio.Shutdown(ctx); err != nil {
		log.Printf("audio shutdown: %v", err)
	}
}
