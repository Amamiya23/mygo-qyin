package app

import (
	"context"
	"time"

	"lilt/internal/qqmusic"
)

var readSongFans = qqmusic.SongFansNumberContext

// songFansState shares the current song's fan count across playback views.
// It refreshes when the track or account changes, or a favorite update succeeds.
type songFansState struct {
	id     int64
	show   string
	gen    uint64
	cancel context.CancelFunc
}

func (a *app) syncSongFans() {
	var id int64
	if tr := a.pl.Current(); tr != nil && tr.ProgramID == 0 {
		id = tr.ID
	}
	if a.quitting || a.win == nil || id == a.songFans.id {
		return
	}
	a.resetSongFans()
	a.songFans.id = id
	if id == 0 {
		return
	}
	a.loadSongFans()
}

func (a *app) resetSongFans() {
	if a.songFans.cancel != nil {
		a.songFans.cancel()
	}
	a.songFans = songFansState{gen: a.songFans.gen + 1}
}

func (a *app) loadSongFans() {
	id, gen := a.songFans.id, a.songFans.gen
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	a.songFans.cancel = cancel
	fetch := readSongFans
	go func() {
		defer cancel()
		show, err := fetch(ctx, id)
		a.post(func() {
			if a.quitting || a.songFans.id != id || a.songFans.gen != gen {
				return
			}
			a.songFans.cancel = nil
			if err == nil {
				a.songFans.show = show
			}
		})
	}()
}

func (a *app) refreshSongFans(id int64) {
	tr := a.pl.Current()
	if a.quitting || a.win == nil || tr == nil || tr.ProgramID != 0 || tr.ID != id {
		return
	}
	if a.songFans.id != id {
		a.syncSongFans()
		return
	}
	if a.songFans.cancel != nil {
		a.songFans.cancel()
	}
	a.songFans.gen++
	a.loadSongFans()
}
