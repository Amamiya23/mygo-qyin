package main

import (
	"math/rand/v2"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/mp3"
)

type repeatMode int

const (
	repeatOff repeatMode = iota
	repeatAll
	repeatOne
)

// player plays a queue of tracks. Its methods run on the main thread;
// loading happens on goroutines, whose results post brings back there.
type player struct {
	post func(func())
	// quality returns the stream level to load, as the settings say.
	quality func() string

	queue   []Track
	order   []int // the queue's indices in play order: shuffled or not
	at      int   // where in order the current track is, -1 for none
	shuffle bool
	repeat  repeatMode
	volume  float64

	track   *Track
	out     *audio.Player
	stream  *mp3Stream
	loading bool
	playing bool // what the listener wants: false while paused
	err     string
	gen     int // counts loads, to drop the results of stale ones

	lyrics       []LyricLine
	lyricsLoaded bool

	// pending holds a seek asked for while the track loads.
	pending time.Duration
}

func newPlayer(post func(func())) *player {
	p := &player{post: post, at: -1, volume: 0.8}
	// The media keys, and the controls of Control Center.
	mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
		switch cmd {
		case mygo.MediaPlay, mygo.MediaPause:
			if (cmd == mygo.MediaPlay) != p.playing {
				p.togglePlay()
			}
		case mygo.MediaNext:
			p.next()
		case mygo.MediaPrevious:
			p.prev()
		case mygo.MediaSeek:
			p.seek(pos)
		}
		p.post(func() {})
	})
	return p
}

// sync shows the system where playback is.
func (p *player) sync() {
	if p.track == nil {
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.SetPlayback(state, p.position())
}

// announce shows the system the track, and its cover once loaded.
func (p *player) announce(t *Track, artwork []byte) {
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.Set(mygo.NowPlayingInfo{
		Title:    t.Name,
		Artist:   t.ArtistNames(),
		Album:    t.Album.Name,
		Artwork:  artwork,
		Duration: t.Length(),
		Position: p.position(),
		State:    state,
		Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
	})
}

func (p *player) ended() {
	if p.repeat == repeatOne {
		p.out.Seek(0)
		p.out.Play()
		return
	}
	if p.at+1 >= len(p.order) && p.repeat == repeatOff {
		p.playing = false
		p.sync()
		return
	}
	p.next()
}

// playQueue replaces the queue with tracks and plays the one at start.
func (p *player) playQueue(tracks []Track, start int) {
	p.queue = append([]Track(nil), tracks...)
	p.order = make([]int, len(p.queue))
	for i := range p.order {
		p.order[i] = i
	}
	if p.shuffle {
		p.shuffleOrder(start)
		p.load(0)
		return
	}
	p.load(start)
}

// shuffleOrder shuffles the order, with first first.
func (p *player) shuffleOrder(first int) {
	rand.Shuffle(len(p.order), func(i, j int) { p.order[i], p.order[j] = p.order[j], p.order[i] })
	for i, v := range p.order {
		if v == first {
			p.order[0], p.order[i] = p.order[i], p.order[0]
		}
	}
}

func (p *player) toggleShuffle() {
	p.shuffle = !p.shuffle
	if len(p.order) == 0 {
		return
	}
	cur := p.order[max(p.at, 0)]
	if p.shuffle {
		p.shuffleOrder(cur)
		p.at = 0
	} else {
		for i := range p.order {
			p.order[i] = i
		}
		p.at = cur
	}
}

func (p *player) cycleRepeat() { p.repeat = (p.repeat + 1) % 3 }

// current returns the track playing or loading, or nil.
func (p *player) current() *Track { return p.track }

// upNext returns the tracks after the current one, in play order.
func (p *player) upNext(n int) []int {
	var out []int
	for i := p.at + 1; i < len(p.order) && len(out) < n; i++ {
		out = append(out, p.order[i])
	}
	return out
}

func (p *player) next() {
	if len(p.order) == 0 {
		return
	}
	at := p.at + 1
	if at >= len(p.order) {
		at = 0
	}
	p.load(at)
}

func (p *player) prev() {
	if len(p.order) == 0 {
		return
	}
	// As every player does: the start of this track, then the one before.
	if p.position() > 3*time.Second {
		p.seek(0)
		return
	}
	at := p.at - 1
	if at < 0 {
		at = len(p.order) - 1
	}
	p.load(at)
}

// jump plays the queue's track i.
func (p *player) jump(i int) {
	for k, v := range p.order {
		if v == i {
			p.load(k)
			return
		}
	}
}

func (p *player) togglePlay() {
	if p.out == nil {
		if p.track == nil && len(p.order) > 0 {
			p.load(max(p.at, 0))
		}
		return
	}
	if p.playing {
		p.out.Pause()
		p.playing = false
	} else {
		if !p.out.Playing() && p.out.Position() >= p.out.Duration()-time.Second {
			p.out.Seek(0)
		}
		p.out.Play()
		p.playing = true
	}
	p.sync()
}

func (p *player) seek(t time.Duration) {
	if p.out == nil {
		p.pending = t
		return
	}
	p.out.Seek(t)
	if p.playing {
		p.out.Play()
	}
	p.sync()
}

func (p *player) setVolume(v float64) {
	p.volume = v
	if p.out != nil {
		p.out.SetVolume(v * v) // the ear hears loudness in powers
	}
}

func (p *player) position() time.Duration {
	if p.out == nil {
		return p.pending
	}
	return p.out.Position()
}

func (p *player) duration() time.Duration {
	if p.track == nil {
		return 0
	}
	return p.track.Length()
}

// levels returns the spectrum of what plays now.
func (p *player) levels() ([nBands]float32, bool) {
	if p.stream == nil || p.out == nil || !p.playing {
		return [nBands]float32{}, false
	}
	frame := int64(p.out.Position().Seconds() * float64(p.stream.rate))
	return p.stream.spec.at(frame)
}

func (p *player) stop() {
	if p.out != nil {
		p.out.Close()
	}
	p.out, p.stream = nil, nil
}

// load plays the track at order[at], skipping those that cannot play.
func (p *player) load(at int) {
	p.loadSkipping(at, 0)
}

func (p *player) loadSkipping(at, skipped int) {
	p.stop()
	p.gen++
	gen := p.gen
	p.at = at
	t := p.queue[p.order[at]]
	p.track = &t
	p.loading, p.playing, p.err = true, true, ""
	p.lyrics, p.lyricsLoaded = nil, false
	p.pending = 0
	p.announce(&t, nil)

	go func() {
		cover, err := fetchBytes(sized(t.Album.PicURL, 600))
		if err == nil {
			p.post(func() {
				if gen == p.gen {
					p.announce(&t, cover)
				}
			})
		}
	}()

	go func() {
		lrc, trans, err := Lyrics(t.ID)
		p.post(func() {
			if gen == p.gen {
				if err == nil {
					p.lyrics = mergeLyrics(lrc, trans)
				}
				p.lyricsLoaded = true
			}
		})
	}()
	go func() {
		level := "exhigh"
		if p.quality != nil {
			level = p.quality()
		}
		u, size, err := SongURL(t.ID, level)
		var s *mp3Stream
		if err == nil {
			dl := startDownload(u, size)
			s, err = openMP3Stream(dl, t.Duration)
			if err != nil {
				dl.Close()
			}
		}
		p.post(func() {
			if gen != p.gen {
				if s != nil {
					s.Close()
				}
				return
			}
			p.loading = false
			if err != nil {
				p.err = err.Error()
				if err == errUnavailable {
					p.err = "这首歌需要 VIP 才能播放"
				}
				// Skip what cannot play, unless nothing in the queue can.
				if skipped+1 < len(p.order) {
					next := at + 1
					if next >= len(p.order) {
						next = 0
					}
					p.loadSkipping(next, skipped+1)
				} else {
					p.playing = false
					p.sync()
				}
				return
			}
			p.stream = s
			p.out = audio.NewPlayer(s)
			p.out.SetVolume(p.volume * p.volume)
			p.out.OnEnd(func() {
				p.post(func() {
					if gen == p.gen {
						p.ended()
					}
				})
			})
			if p.pending > 0 {
				p.out.Seek(p.pending)
				p.pending = 0
			}
			if p.playing {
				p.out.Play()
			}
			p.sync()
		})
	}()
}
