package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

// Where playback was left is kept in session.json in the user data
// directory: on quitting, and every little while as it plays, in case the
// app does not get to quit.

func sessionPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

// saveSession keeps where playback is, if it moved on since last kept.
func (a *app) saveSession() {
	if a.quitting {
		defer a.sessionWrites.wait()
	}
	s, ok := a.pl.Session()
	if !ok {
		return
	}
	path, err := sessionPath()
	if err != nil {
		return
	}
	a.sessionWrites.submit(path, s)
}

type sessionSave struct {
	path  string
	state player.Session
}

// One worker writes snapshots in order, replacing queued work with the latest
// snapshot. Shutdown waits on done without requiring a main-thread callback.
type sessionWriter struct {
	mu      sync.Mutex
	pending *sessionSave
	done    chan struct{}
	saved   string
}

func (w *sessionWriter) submit(path string, s player.Session) <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = &sessionSave{path, s}
	if w.done == nil {
		w.done = make(chan struct{})
		go w.run()
	}
	return w.done
}

func (w *sessionWriter) wait() {
	w.mu.Lock()
	done := w.done
	w.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (w *sessionWriter) run() {
	for {
		w.mu.Lock()
		job, saved := w.pending, w.saved
		if job == nil {
			close(w.done)
			w.done = nil
			w.mu.Unlock()
			return
		}
		w.pending = nil
		w.mu.Unlock()

		b, err := json.Marshal(job.state)
		if err != nil || string(b) == saved {
			continue
		}
		if os.WriteFile(job.path+".part", b, 0o600) != nil || os.Rename(job.path+".part", job.path) != nil {
			continue
		}
		w.mu.Lock()
		w.saved = string(b)
		w.mu.Unlock()
	}
}

// sessionEvery is how long after playback changed, and how often as it
// plays, the session is kept.
const sessionEvery = 30 * time.Second

// keepSession saves the session a while after playback changed, and every
// while again as it plays. Paused, nothing is due, and the app sleeps.
func (a *app) keepSession() {
	if a.sessionDue {
		return
	}
	a.sessionDue = true
	sessionAfter(sessionEvery, func() {
		a.sessionDue = false
		a.saveSession()
		if a.pl.Playing() {
			a.keepSession()
		}
	})
}

// sessionAfter runs fn on the main thread after d, without redrawing a
// window as Update would.
var sessionAfter = func(d time.Duration, fn func()) {
	time.AfterFunc(d, func() { mygo.RunOnMain(fn) })
}

// restoreSession picks up playback where it was left, paused, once the
// listener's session is back: the songs they may play depend on it.
func (a *app) restoreSession() {
	if a.restored {
		return
	}
	a.restored = true
	path, err := sessionPath()
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s player.Session
	if json.Unmarshal(b, &s) != nil {
		return
	}
	a.sessionWrites.mu.Lock()
	a.sessionWrites.saved = string(b)
	a.sessionWrites.mu.Unlock()
	qqmusic.RememberTracks(s.Queue)
	a.pl.Restore(s)
	if s.Radio != "" && a.pl.Current() != nil {
		a.pl.AttachRadio(s.Radio, a.radioFeed(s.Radio))
	}
}
