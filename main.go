package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type pageKind int

const (
	pageHome pageKind = iota
	pageDaily
	pageCharts
	pagePlaylist
	pageSearch
)

type route struct {
	page     pageKind
	playlist int64
}

// app is the state both windows show. Everything in it is read and changed
// on the main thread: by the views, and by goroutines through post.
type app struct {
	win   *mygo.Window
	notch *notch
	art   *artCache
	pl    *player
	pal   palette

	route   route
	history []route
	forward []route // the pages gone back from, the last first to go to

	profile      *Profile
	mine, saved  []Playlist // the listener's playlists: made, and saved
	recPlaylists []Playlist // the day's playlists picked for the listener
	login        loginState
	liked        map[int64]bool // the songs the listener likes
	toast        string         // a message to show once

	daily       []Track
	featured    []Playlist
	charts      []Playlist
	homeErr     string
	homeLoading bool

	playlists map[int64]*Playlist
	loadingPl map[int64]bool
	plErr     map[int64]string
	trackList ui.ListState

	query     string
	searched  string
	results   []Track
	searching bool
	searchErr string
	searchBox ui.Handle

	nowPlaying  bool
	npOpen      bool      // Now Playing as it last showed: open or not
	npChangedAt time.Time // when it last opened or closed
	npPane      int       // 0: lyrics, 1: up next
	lyricScroll ui.ScrollState
	lyricAt     int
	lyricTrack  int64

	scrub      float64
	scrubbing  bool
	volume     float64
	volumeOpen bool // the volume popover of the player bar shows
	mutedFrom  float64

	rowBars barState
	// underTop and underBottom say whether the page has content under its
	// bars, which show their scroll edges then.
	underTop, underBottom bool
	barScrub              scrubState
	npScrub               scrubState
	backdropCache         backdropCache // the backdrop of Now Playing, as last painted
	sig                   playerSig
	quitting              bool

	settings    settings
	settingsWin *mygo.Window
}

// post runs fn on the main thread and redraws both windows. Without a
// window, as in tests, it drops fn.
func (a *app) post(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		fn()
		a.redraw()
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.notch != nil && a.notch.win != nil {
		a.notch.win.Invalidate()
	}
}

// shotHook, when set, drives the app to check its design.
var shotHook func(a *app)

func (a *app) showMain() {
	if a.win != nil {
		a.win.Show()
		a.win.Focus()
	}
}

func main() {
	a := &app{
		playlists: map[int64]*Playlist{},
		loadingPl: map[int64]bool{},
		plErr:     map[int64]string{},
		lyricAt:   -1,
	}
	mygo.App.SetName("网易云音乐")
	mygo.App.WhenReady(func() {
		a.settings = loadSettings()
		a.settings.applyAppearance()
		a.npPane = a.lyricsPaneChosen()
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         "网易云音乐",
			Width:         1180,
			Height:        780,
			MinWidth:      880,
			MinHeight:     580,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHidden,
			// Inset from the corner, within the sidebar, as macOS 26's
			// apps with a sidebar place them.
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			Vibrancy:             mygo.VibrancySidebar,
			BackgroundColor:      "light-dark(#ffffff, #141416)",
			Content:              ui.View(a.view),
		})
		a.art = newArtCache(a.post)
		a.pl = newPlayer(a.post)
		a.pl.quality = func() string { return a.settings.Quality }
		a.volume = a.settings.Volume
		a.pl.setVolume(a.volume)
		a.setMenu()
		a.boot()
		a.notch = newNotch(a)
		if shotHook != nil {
			shotHook(a)
		}
		// Closing the window hides it: music plays on, under the notch,
		// and the Dock icon brings it back.
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				a.win.Hide()
			}
		})
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.settings.Volume = a.volume
		a.settings.save()
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { a.showMain() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
