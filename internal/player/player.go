package player

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatAll
	RepeatOne
)

const (
	NormalizeOff      = "off"
	NormalizeQuiet    = "quiet"
	NormalizeStandard = "standard"
	NormalizeHigh     = "high"
)

// Player plays a queue of tracks. Its methods run on the main thread;
// loading happens on goroutines, whose results post brings back there.
type Player struct {
	post func(func())
	// Quality returns the stream level to load, as the settings say.
	Quality func() string
	// Normalize returns the volume-balance mode, as the settings say.
	Normalize func() string
	// PlaybackFade reports whether playback operations should fade.
	PlaybackFade func() bool
	// Cache keeps the songs played, if set.
	Cache *Cache
	// Changed, if set, hears that what plays, where it is or whether it
	// plays changed.
	Changed func()
	// SkipAutomatic, if set, excludes tracks from automatic selection while
	// leaving them in the queue for direct selection. It runs on the main thread.
	SkipAutomatic func(*qqmusic.Track) bool

	queue   []qqmusic.Track
	order   []int // the queue's indices in play order: shuffled or not
	at      int   // where in order the current track is, -1 for none
	shuffle bool
	repeat  RepeatMode
	volume  float64

	// A radio goes on with the tracks its feed gives, as the queue nears
	// its end, for as long as nothing else is played.
	radio    string
	feed     func(got func([]qqmusic.Track))
	radioGen int  // counts radios, to drop what an old one gives
	feeding  bool // the feed is asked
	awaiting bool // at the queue's end, for what the feed gives

	track           *qqmusic.Track
	out             *audio.Player
	stream          stream
	info            *qqmusic.Stream // what the current track streams
	fade            fader
	loading         bool
	playing         bool // what the listener wants: false while paused
	selectedSkipped bool // the listener explicitly selected a skipped track
	err             string
	gen             int // counts loads, to drop the results of stale ones
	cancelLoad      context.CancelFunc

	lyrics       []lyrics.Line
	lyricsLoaded bool
	instrumental bool

	// pending holds a seek asked for while the track loads.
	pending time.Duration
	// A seek playing fades out, then lands, then fades in: meanwhile the
	// position is where it goes.
	seeking bool
	seekTo  time.Duration
	seekGen int
}

func New(post func(func())) *Player {
	p := &Player{post: post, at: -1, volume: 0.8}
	// Music, mixed far enough ahead to play on while macOS keeps the app
	// waiting, as it does while its appearance changes.
	audio.SetLatency(audio.LatencyPlayback)
	// The media keys, and the controls of Control Center.
	mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
		switch cmd {
		case mygo.MediaPlay, mygo.MediaPause:
			if (cmd == mygo.MediaPlay) != p.playing {
				p.TogglePlay()
			}
		case mygo.MediaNext:
			p.Next()
		case mygo.MediaPrevious:
			p.Prev()
		case mygo.MediaSeek:
			p.Seek(pos)
		}
		p.post(func() {})
	})
	return p
}

// sync shows the system, and Changed, where playback is.
func (p *Player) sync() {
	if p.Changed != nil {
		defer p.Changed()
	}
	if p.track == nil {
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.SetPlayback(state, p.Position())
}

// announce shows the system the track, and its cover once loaded.
func (p *Player) announce(t *qqmusic.Track, artwork []byte) {
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
		Position: p.Position(),
		State:    state,
		Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
	})
}

func (p *Player) ended() {
	if p.repeat == RepeatOne && !p.skips(p.track) {
		p.out.Seek(0)
		p.out.Play()
		return
	}
	at := p.automaticAt(p.at, 1, p.repeat == RepeatAll && p.feed == nil)
	switch {
	case at >= 0:
		p.load(at)
	case p.feed != nil:
		p.awaitFeed()
	default:
		p.playing = false
		p.sync()
	}
}

// PlayQueue replaces the queue with tracks and plays the one at start.
func (p *Player) PlayQueue(tracks []qqmusic.Track, start int) {
	if len(tracks) == 0 {
		return
	}
	p.endRadio()
	start = min(max(start, 0), len(tracks)-1)
	p.queue = append([]qqmusic.Track(nil), tracks...)
	p.order = make([]int, len(p.queue))
	for i := range p.order {
		p.order[i] = i
	}
	if p.shuffle {
		p.shuffleOrder(start)
		p.loadSelected(0)
		return
	}
	p.loadSelected(start)
}

// PlayNext inserts t after the current track in play order without changing
// playback. With nothing playing, it queues t for when playback starts.
func (p *Player) PlayNext(t qqmusic.Track) {
	i := len(p.queue)
	p.queue = append(p.queue, t)
	// A menu can insert while the queue view is being built. Keep its copy
	// of the old order intact until the next build.
	p.order = slices.Insert(slices.Clone(p.order), p.at+1, i)
	p.sync()
}

// Remove deletes the queue's track i without mutating previously returned
// slices so an in-progress queue build finishes cleanly.
func (p *Player) Remove(i int) {
	if i < 0 || i >= len(p.queue) {
		return
	}
	pos := -1
	for k, v := range p.order {
		if v == i {
			pos = k
			break
		}
	}
	if pos < 0 {
		return
	}
	newQueue := slices.Delete(slices.Clone(p.queue), i, i+1)
	newOrder := make([]int, 0, len(p.order)-1)
	for k, v := range p.order {
		if k == pos {
			continue
		}
		if v > i {
			v--
		}
		newOrder = append(newOrder, v)
	}
	p.queue, p.order = newQueue, newOrder
	if len(p.queue) == 0 {
		p.stop()
		p.gen++
		p.endRadio()
		p.at = -1
		p.track = nil
		p.loading, p.playing, p.err = false, false, ""
		p.lyrics, p.lyricsLoaded, p.instrumental = nil, false, false
		p.pending = 0
		p.sync()
		return
	}
	if p.at < 0 {
		p.sync()
		return
	}
	switch {
	case pos < p.at:
		p.at--
		p.sync()
	case pos > p.at:
		p.refill()
		p.sync()
	default:
		wasPlaying := p.playing
		nextAt := p.automaticAt(pos-1, 1, true)
		if nextAt < 0 {
			nextAt, wasPlaying = pos%len(p.order), false
		}
		p.loadSkipping(nextAt, 0, wasPlaying, 0, false)
	}
}

// PlayRadio plays tracks in their order, then goes on with those feed
// gives as the queue nears its end, until something else is played. The
// radio's name says which it is. feed calls got on the main thread.
func (p *Player) PlayRadio(name string, tracks []qqmusic.Track, feed func(got func([]qqmusic.Track))) {
	first := slices.IndexFunc(tracks, func(t qqmusic.Track) bool { return !p.skips(&t) })
	if first < 0 {
		return
	}
	shuffle := p.shuffle
	p.shuffle = false
	p.PlayQueue(tracks, first)
	p.shuffle = shuffle
	p.AttachRadio(name, feed)
}

// RadioFrom makes the track playing the first of a radio's queue, which
// goes on with tracks, then with those feed gives: without starting it
// over.
func (p *Player) RadioFrom(name string, tracks []qqmusic.Track, feed func(got func([]qqmusic.Track))) {
	if p.track == nil {
		return
	}
	p.queue, p.order, p.at = []qqmusic.Track{*p.track}, []int{0}, 0
	p.appendTracks(tracks)
	p.AttachRadio(name, feed)
}

// AttachRadio makes the queue the radio name's, which feed goes on with,
// as after restoring a session that played it.
func (p *Player) AttachRadio(name string, feed func(got func([]qqmusic.Track))) {
	p.endRadio()
	p.radio, p.feed = name, feed
	p.refill()
}

// EndRadio stops the radio playing: the song playing plays on, but the
// songs the radio queued after it go, and it adds no more.
func (p *Player) EndRadio() {
	if p.radio == "" && p.feed == nil {
		return
	}
	awaiting := p.awaiting
	p.endRadio()
	if p.at >= 0 && p.at+1 < len(p.order) {
		kept := make([]qqmusic.Track, p.at+1)
		for i, k := range p.order[:p.at+1] {
			kept[i] = p.queue[k]
		}
		p.queue, p.order = kept, make([]int, len(kept))
		for i := range p.order {
			p.order[i] = i
		}
	}
	if awaiting {
		// At its end, waiting for more: nothing plays on.
		p.playing = false
		p.sync()
	}
}

func (p *Player) endRadio() {
	p.radioGen++
	p.radio, p.feed, p.feeding, p.awaiting = "", nil, false, false
}

// Radio returns the name of the radio playing, or "".
func (p *Player) Radio() string { return p.radio }

// refill asks the radio for more once the queue is near its end.
func (p *Player) refill() {
	if p.feed == nil || p.feeding {
		return
	}
	// Retained, skipped songs do not count as buffered playback.
	if next := p.automaticAt(p.at, 1, false); next >= 0 && p.automaticAt(next, 1, false) >= 0 {
		return
	}
	p.feeding = true
	gen := p.radioGen
	p.feed(func(more []qqmusic.Track) {
		if gen != p.radioGen {
			return
		}
		p.feeding = false
		p.appendTracks(more)
		next := p.automaticAt(p.at, 1, false)
		switch {
		case !p.awaiting:
		case next >= 0:
			p.awaiting = false
			p.load(next)
		default:
			// The radio has nothing more: it stops at its end.
			p.awaiting, p.playing = false, false
			p.err = "电台暂时没有更多歌曲"
			p.sync()
		}
	})
}

// awaitFeed waits at the end of a radio's queue for what its feed gives.
func (p *Player) awaitFeed() {
	p.awaiting = true
	p.refill()
}

// appendTracks adds tracks to the end of the queue, but those in it.
func (p *Player) appendTracks(tracks []qqmusic.Track) {
	in := make(map[int64]bool, len(p.queue))
	for _, t := range p.queue {
		in[t.ID] = true
	}
	for _, t := range tracks {
		if !in[t.ID] {
			in[t.ID] = true
			p.order = append(p.order, len(p.queue))
			p.queue = append(p.queue, t)
		}
	}
}

// shuffleOrder shuffles the order, with first first.
func (p *Player) shuffleOrder(first int) {
	rand.Shuffle(len(p.order), func(i, j int) { p.order[i], p.order[j] = p.order[j], p.order[i] })
	for i, v := range p.order {
		if v == first {
			p.order[0], p.order[i] = p.order[i], p.order[0]
		}
	}
}

func (p *Player) ToggleShuffle() {
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

func (p *Player) CycleRepeat() { p.repeat = (p.repeat + 1) % 3 }

// Current returns the track playing or loading, or nil.
func (p *Player) Current() *qqmusic.Track { return p.track }

// Queue returns the tracks queued, in the order they were given.
func (p *Player) Queue() []qqmusic.Track { return p.queue }

// Playing reports what the listener wants: false while paused.
func (p *Player) Playing() bool { return p.playing }

// Loading reports whether playback is waiting for a track or for its audio.
func (p *Player) Loading() bool { return p.loading || p.awaiting || p.playing && p.buffering() }

func (p *Player) buffering() bool { return p.out != nil && p.out.Buffering() }

// Ready reports whether the current track has loaded, to play.
func (p *Player) Ready() bool { return p.out != nil }

// Err returns why the current track did not play, or "".
func (p *Player) Err() string { return p.err }

// ClearErr clears a pending playback error, as when account recovery succeeds.
func (p *Player) ClearErr() {
	if p.err == "" {
		return
	}
	p.err = ""
	p.sync()
}

// Stream returns what the current track streams, once loaded, or nil.
func (p *Player) Stream() *qqmusic.Stream { return p.info }

func (p *Player) Shuffle() bool      { return p.shuffle }
func (p *Player) Repeat() RepeatMode { return p.repeat }

// Lyrics returns the current track's lyrics, and whether they have loaded.
// Published lines are read-only; loading another result replaces the slice.
func (p *Player) Lyrics() ([]lyrics.Line, bool) { return p.lyrics, p.lyricsLoaded }

// Instrumental reports an explicit instrumental response for the current track.
func (p *Player) Instrumental() bool { return p.instrumental }

// Downloaded returns the share of the current track downloaded so far, or
// false before it has started to.
func (p *Player) Downloaded() (float64, bool) {
	if p.stream == nil {
		return 0, false
	}
	dl, _ := p.stream.source()
	return dl.Progress(), true
}

// Audio is the current track's sound decoded apart from what plays, as
// OpenAudio opens it: interleaved stereo at its rate, read as it
// downloads.
type Audio interface {
	SampleRate() int
	Channels() int
	Read(dst []float32) (int, error)
	// SetPosition goes to a frame, of SampleRate a second.
	SetPosition(frame int64) error
	Close() error
}

// OpenAudio opens the current track's sound again, for decoding apart
// from what plays, as for transcribing an episode. It reads the same
// download, waiting for what has not come in, and fails once the track
// changes. It works on any goroutine once opened; close it when done.
func (p *Player) OpenAudio() (Audio, error) {
	if p.stream == nil || p.track == nil {
		return nil, errors.New("player: nothing loaded")
	}
	dl, _ := p.stream.source()
	return openShared(dl, p.track.Duration)
}

// PlayOrder returns the queue's indices in play order, and where the
// current track is among them, -1 for none.
func (p *Player) PlayOrder() ([]int, int) { return p.order, p.at }

func (p *Player) skips(tr *qqmusic.Track) bool {
	return tr != nil && p.SkipAutomatic != nil && p.SkipAutomatic(tr)
}

// automaticAt searches the existing play order without changing the visible queue.
func (p *Player) automaticAt(at, step int, wrap bool) int {
	for range p.order {
		at += step
		if at < 0 || at >= len(p.order) {
			if !wrap {
				break
			}
			at = (at + len(p.order)) % len(p.order)
		}
		if !p.skips(&p.queue[p.order[at]]) {
			return at
		}
	}
	return -1
}

// Upcoming returns the next automatically playable track without wrapping.
func (p *Player) Upcoming() *qqmusic.Track {
	if at := p.automaticAt(p.at, 1, false); at >= 0 {
		return &p.queue[p.order[at]]
	}
	return nil
}

func (p *Player) Next() { p.next(true) }

// NextWithoutWrap advances within the current queue, waiting for more on
// a radio. At the end of a finite queue, it pauses the current track.
func (p *Player) NextWithoutWrap() {
	p.selectedSkipped = false
	p.next(false)
}

func (p *Player) next(wrap bool) {
	if len(p.order) == 0 {
		return
	}
	at := p.automaticAt(p.at, 1, wrap && p.feed == nil)
	switch {
	case at >= 0:
		p.load(at)
	case p.feed != nil:
		p.stop()
		if p.loading {
			p.gen++ // A reply for the rejected track must not start it again.
			p.loading = false
		}
		p.awaitFeed()
	case p.playing:
		if p.out != nil {
			p.TogglePlay()
		} else {
			p.playing = false
			p.sync()
		}
	}
}

// Prev plays the track before this one, however far this one has played:
// once a few seconds in, going back to its start only kept the listener
// pressing twice.
func (p *Player) Prev() {
	if at := p.automaticAt(max(p.at, 0), -1, true); at >= 0 {
		p.load(at)
	}
}

// Jump plays the queue's track i.
func (p *Player) Jump(i int) {
	for k, v := range p.order {
		if v == i {
			p.loadSelected(k)
			return
		}
	}
}

// TogglePlay pauses, fading out, or plays, fading in.
func (p *Player) TogglePlay() {
	if !p.playing && !p.selectedSkipped && p.skips(p.track) {
		p.next(false)
		return
	}
	if p.out == nil {
		switch {
		case p.loading:
			// It plays once loaded, or not.
			p.playing = !p.playing
			p.sync()
		case len(p.order) > 0:
			p.loadSkipping(max(p.at, 0), 0, true, p.pending, p.selectedSkipped)
		}
		return
	}
	out := p.out
	if p.playing && out.Buffering() {
		p.playing = false
		out.Pause()
		p.sync()
		return
	}
	if p.PlaybackFade != nil && !p.PlaybackFade() {
		p.fade.attach(out, 1)
		if p.playing {
			p.playing = false
			out.Pause()
		} else {
			p.playing = true
			p.resume(out)
		}
		p.sync()
		return
	}
	if p.playing {
		p.playing = false
		p.fade.fade(0, FadeOutDuration, func() {
			p.post(func() {
				if p.out == out && !p.playing {
					out.Pause()
					p.sync()
				}
			})
		})
		p.sync()
		return
	}
	p.playing = true
	p.resume(out)
	p.fade.fade(1, fadeIn, nil)
	p.sync()
}

func (p *Player) resume(out *audio.Player) {
	if p.seeking || out.Playing() {
		return
	}
	if duration := out.Duration(); duration > 0 && !out.Buffering() && out.Position() >= duration-time.Second {
		out.Seek(0)
	}
	out.Play()
}

// seekGap is how long a seek waits for what plays to fade out, which the
// audio player does over 5 ms, as it pauses: within the next buffer or
// two it sends the system.
const seekGap = 30 * time.Millisecond

// Seek moves to t. Playing, it pauses and plays again around the jump,
// which the audio player fades over a few milliseconds, rather than
// cutting from one sound to the other with a click.
func (p *Player) Seek(t time.Duration) {
	if p.out == nil {
		p.pending = t
		return
	}
	out := p.out
	// Paused, it jumps at once; but a seek on its way paused it only for
	// the jump, and this one takes its place, playing on from there.
	if !out.Playing() && !p.seeking {
		out.Seek(t)
		p.sync()
		return
	}
	p.seekGen++
	gen := p.seekGen
	p.seeking, p.seekTo = true, t
	out.Pause()
	time.AfterFunc(seekGap, func() {
		p.post(func() {
			if gen != p.seekGen || p.out != out {
				return
			}
			p.seeking = false
			out.Seek(t)
			if p.playing || !out.Buffering() && !p.fade.silent() {
				out.Play()
			}
			p.sync()
		})
	})
	p.sync()
}

func (p *Player) SetVolume(v float64) {
	p.volume = v
	p.applyGain()
}

// RefreshGain applies a changed normalization setting to the current song.
func (p *Player) RefreshGain() {
	p.applyGain()
	if p.Normalize == nil || p.Normalize() == NormalizeOff || p.info == nil || p.track == nil {
		return
	}
	// Enabling mid-song fetches measurements without restarting playback.
	t, gen := *p.track, p.gen
	fetch := songLoudness
	go func() {
		v, err := fetch(t)
		if err != nil {
			return
		}
		p.post(func() {
			if gen == p.gen && p.info != nil {
				p.info = withLoudness(p.info, v)
				p.applyGain()
			}
		})
	}()
}

// applyGain sets the volume of what plays: the listener's, which the ear
// hears in powers, and the track's gain, if normalizing.
func (p *Player) applyGain() {
	p.fade.setGain(p.gain())
}

func (p *Player) gain() float64 {
	g := p.volume * p.volume
	if p.Normalize != nil {
		if mode := p.Normalize(); mode != NormalizeOff {
			g *= normGain(p.info, mode)
		}
	}
	return g
}

func (p *Player) applyLateLoudness(gen int, v qqmusic.Loudness) {
	if gen != p.gen || p.info == nil {
		return
	}
	p.info = withLoudness(p.info, v)
	p.fade.rampGain(p.gain(), 200*time.Millisecond)
}

// normPreamp lifts balanced playback before peak protection.
func normPreamp(mode string) float64 {
	switch mode {
	case NormalizeQuiet:
		return 1
	case NormalizeStandard:
		return 4
	case NormalizeHigh:
		return 7
	}
	return 0
}

// normGain adds the mode's preamp to QQ's measured gain, limiting it so peaks
// cannot clip.
func normGain(st *qqmusic.Stream, mode string) float64 {
	if st == nil || st.Gain == 0 && st.Peak == 0 {
		return 1
	}
	g := math.Pow(10, (st.Gain+normPreamp(mode))/20)
	if st.Peak > 0 {
		g = min(g, 1/st.Peak)
	}
	return g
}

func (p *Player) Position() time.Duration {
	switch {
	case p.out == nil:
		return p.pending
	case p.seeking:
		return p.seekTo
	}
	return p.out.Position()
}

func (p *Player) Duration() time.Duration {
	if p.track == nil {
		return 0
	}
	return p.track.Length()
}

// Levels returns the spectrum of what plays now.
func (p *Player) Levels() ([Bands]float32, bool) {
	if p.stream == nil || p.out == nil || !p.playing {
		return [Bands]float32{}, false
	}
	_, spec := p.stream.source()
	frame := int64(p.Position().Seconds() * float64(p.stream.SampleRate()))
	bands, _ := spec.at(frame)
	// An uncached chunk is measured asynchronously. Keep the visualizer live
	// while it warms up so its next frame can pick up that result.
	return bands, spec != nil
}

func (p *Player) stop() {
	if p.cancelLoad != nil {
		// Also wakes a decoder still waiting for the first bytes of a download.
		p.cancelLoad()
		p.cancelLoad = nil
	}
	if out := p.out; out != nil {
		// Paused first, it fades out over a few milliseconds, rather than
		// stopping with a click.
		out.Pause()
		time.AfterFunc(100*time.Millisecond, out.Close)
	}
	p.out, p.stream, p.info = nil, nil, nil
	p.seeking = false
	p.fade.attach(nil, 1)
}

// load plays the track at order[at], skipping those that cannot play.
func (p *Player) load(at int) {
	p.loadSkipping(at, 0, true, 0, false)
}

func (p *Player) loadSelected(at int) {
	p.loadSkipping(at, 0, true, 0, p.skips(&p.queue[p.order[at]]))
}

// loadSkipping loads the track at order[at], to play or not, from from,
// skipping those that cannot play: skipped of them so far.
func (p *Player) loadSkipping(at, skipped int, play bool, from time.Duration, selectedSkipped bool) {
	p.stop()
	p.gen++
	gen := p.gen
	p.at = at
	t := p.queue[p.order[at]]
	p.track = &t
	p.selectedSkipped = selectedSkipped
	p.loading, p.playing, p.err = true, play, ""
	p.awaiting = false
	p.lyrics, p.lyricsLoaded = nil, false
	p.instrumental = false
	p.pending = from
	p.announce(&t, nil)
	p.refill()

	cache := p.Cache
	coverURL := qqmusic.Sized(t.Album.PicURL, 600)
	if coverURL != "" {
		go func() {
			var cover []byte
			var err error
			if cached, ok := cache.LookupCover(coverURL); ok {
				cover = cached
			} else {
				cover, err = fetchCover(coverURL)
				if err == nil {
					cache.KeepCover(coverURL, cover)
				}
			}
			if err == nil && len(cover) > 0 {
				p.post(func() {
					if gen == p.gen {
						p.announce(&t, cover)
					}
				})
			}
		}()
	}

	go func() {
		var lrc, trans, timed string
		var err error
		if cached, ok := cache.LookupLyrics(t.ID); ok {
			lrc, trans, timed = cached.LRC, cached.Translation, cached.Timed
		} else {
			lrc, trans, timed, err = songLyrics(t.ID)
			if err == nil {
				cache.KeepLyrics(t.ID, lrc, trans, timed)
			}
		}
		var lines []lyrics.Line
		if err == nil {
			lines = lyrics.Build(lrc, trans, timed)
		}
		instrumental := err == nil && len(lines) == 0 && lyrics.IsInstrumental(lrc)
		p.post(func() {
			if gen == p.gen {
				p.lyrics = lines
				p.lyricsLoaded = true
				p.instrumental = instrumental
			}
		})
	}()
	level := "exhigh"
	if p.Quality != nil {
		level = p.Quality()
	}
	normalize := p.Normalize != nil && p.Normalize() != NormalizeOff
	ctx, cancel := context.WithCancel(context.Background())
	p.cancelLoad = cancel
	resolve := songURL
	go func() {
		info, s, loudness, err := openTrack(ctx, cache, t, level, normalize, resolve)
		if err != nil {
			cancel()
		}
		var policy audio.StreamBuffer
		var dl *download
		if err == nil {
			policy = playbackBuffer(s, info.Level)
			if policy.Start > 0 {
				var spec *spectrum
				dl, spec = s.source()
				if _, complete := dl.completedSize(); !complete {
					spec.reserve(s.SampleRate(), int(policy.Capacity/time.Second)+1)
				}
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
				unavailable := errors.Is(err, qqmusic.ErrUnavailable)
				if unavailable {
					if t.ProgramID != 0 {
						p.err = "这期节目需要购买才能收听"
					} else {
						p.err = "这首歌需要 VIP 才能播放"
					}
				}
				// Skip only per-track unplayable errors while actively trying to play.
				if p.playing && unavailable {
					if next := p.automaticAt(p.at, 1, true); next >= 0 {
						distance := (next - p.at + len(p.order)) % len(p.order)
						if distance > 0 && skipped+distance < len(p.order) {
							p.loadSkipping(next, skipped+distance, true, 0, false)
							return
						}
					}
				}
				p.playing = false
				p.sync()
				return
			}
			p.stream, p.info = s, info
			buffer := audio.StreamBuffer{}
			if dl != nil {
				if _, complete := dl.completedSize(); !complete {
					buffer = policy
					buffer.Changed = func() {
						p.post(func() {
							if gen == p.gen && p.out != nil {
								if !p.playing && p.buffering() {
									p.out.Pause()
								}
								p.sync()
							}
						})
					}
				}
			}
			out := audio.NewBufferedPlayer(s, buffer).SetPrimary(true).SetPreRoll(true)
			p.out = out
			// From its middle, as a session picked up again, it fades in.
			level := 1.0
			if p.pending > 0 && (p.PlaybackFade == nil || p.PlaybackFade()) {
				level = 0
			}
			p.fade.attach(p.out, level)
			if normalize {
				p.applyGain()
			} else {
				p.RefreshGain()
			}
			out.OnEnd(func() {
				p.post(func() {
					if gen != p.gen || p.out != out {
						return
					}
					if dl != nil && out.StreamError() != nil {
						p.pending = out.Position()
						p.stop()
						p.playing, p.err = false, "音频流读取失败"
						p.sync()
						return
					}
					p.ended()
				})
			})
			if p.pending > 0 {
				p.out.Seek(p.pending)
				p.pending = 0
			}
			if p.playing {
				p.out.Play()
				p.fade.fade(1, fadeIn, nil)
			}
			p.sync()
		})
		if loudness != nil {
			select {
			case v := <-loudness:
				if v != nil {
					p.post(func() { p.applyLateLoudness(gen, *v) })
				}
			case <-ctx.Done():
			}
		}
	}()
}

const loudnessWait = 50 * time.Millisecond

var (
	losslessStreamBuffer = audio.StreamBuffer{
		Capacity: 8 * time.Second,
		Start:    5 * time.Second,
		Resume:   6 * time.Second,
	}
	mp3StreamBuffer = audio.StreamBuffer{
		Capacity: 3 * time.Second,
		Start:    time.Second,
		Resume:   time.Second,
	}
)

// playbackBuffer uses the stream actually delivered, including quality fallback.
func playbackBuffer(s stream, level string) audio.StreamBuffer {
	switch s.(type) {
	case *flacStream:
		if qqmusic.Lossless(level) {
			return losslessStreamBuffer
		}
	case *mp3Stream:
		if level == "standard" || level == "exhigh" {
			return mp3StreamBuffer
		}
	}
	return audio.StreamBuffer{}
}

type songResolver func(ctx context.Context, id int64, level, cachedLevel string) (*qqmusic.Stream, error)

const cacheUpgradeWait = 2 * time.Second

func openAudioSource(ctx context.Context, cache *Cache, id int64, level string, resolve songResolver) (*download, *qqmusic.Stream, error) {
	if dl, info, ok := cache.lookup(id, level); ok {
		return dl, info, nil
	}

	// Hold the lower-quality copy open so it is still usable when a slow
	// request ends. Only look online for qualities that would improve on it.
	fallback, local := cache.lookupLower(id, level)
	requestCtx, cancel := ctx, func() {}
	var cachedLevel string
	if fallback != nil {
		cachedLevel = local.Level
		requestCtx, cancel = context.WithTimeout(ctx, cacheUpgradeWait)
	}
	info, err := resolve(requestCtx, id, level, cachedLevel)
	if cause := requestCtx.Err(); cause != nil {
		err = cause
	}
	cancel()
	if cause := ctx.Err(); cause != nil {
		err = cause
	} else if fallback != nil && err != nil && !errors.Is(err, qqmusic.ErrLoginRequired) {
		return fallback, local, nil
	}
	if fallback != nil {
		fallback.Close()
	}
	if err != nil {
		return nil, nil, err
	}
	// The returned quality may now be cached, for example if a concurrent
	// download finished during resolution.
	if dl, metadata, ok := cache.lookupLevel(id, info.Level); ok {
		return dl, metadata, nil
	}
	return cache.download(ctx, id, info), info, nil
}

// openTrack opens track t's audio at level: kept in the cache, or else
// streamed into it, and kept once it has all come in.
func openTrack(ctx context.Context, cache *Cache, t qqmusic.Track, level string, normalize bool, resolve songResolver) (*qqmusic.Stream, stream, <-chan *qqmusic.Loudness, error) {
	// Resolve loudness while the audio opens, including for songs already on
	// disk. A failed metadata request leaves the stream's gain unchanged.
	var loudness <-chan *qqmusic.Loudness
	if normalize {
		result := make(chan *qqmusic.Loudness, 1)
		loudness = result
		fetch := songLoudness
		go func() {
			v, err := fetch(t)
			if err != nil {
				result <- nil
				return
			}
			result <- &v
		}()
	}
	dl, info, err := openAudioSource(ctx, cache, t.ID, level, resolve)
	if err != nil {
		return nil, nil, nil, err
	}
	s, err := openStream(dl, t.Duration)
	if err != nil {
		dl.Close()
		return nil, nil, nil, err
	}
	if loudness != nil {
		timer := time.NewTimer(loudnessWait)
		defer timer.Stop()
		select {
		case v := <-loudness:
			if v != nil {
				info = withLoudness(info, *v)
			}
			loudness = nil
		case <-timer.C:
		}
	}
	return info, s, loudness, nil
}

var (
	fetchCover   = qqmusic.Fetch
	songLyrics   = qqmusic.Lyrics
	songLoudness = qqmusic.SongLoudness
	songURL      = qqmusic.SongURLAbove
)

func withLoudness(info *qqmusic.Stream, v qqmusic.Loudness) *qqmusic.Stream {
	// Downloads keep their own metadata on completion; do not mutate the
	// Stream they may still be reading on their goroutine.
	st := *info
	st.Gain, st.Peak = v.Gain, v.Peak
	return &st
}

// Session is where playback was left, to pick up again.
type Session struct {
	Queue    []qqmusic.Track `json:"queue"`
	Order    []int           `json:"order"`
	At       int             `json:"at"`
	Shuffle  bool            `json:"shuffle"`
	Repeat   RepeatMode      `json:"repeat"`
	Position int64           `json:"position"` // milliseconds
	Radio    string          `json:"radio,omitempty"`
}

// Session returns an independent snapshot, or false with nothing queued.
// Its slices and metadata may be encoded while the queue changes.
func (p *Player) Session() (Session, bool) {
	if p.track == nil || p.at < 0 {
		return Session{}, false
	}
	queue := slices.Clone(p.queue)
	for i := range queue {
		t := &queue[i]
		t.Artists = slices.Clone(t.Artists)
		t.Alias = slices.Clone(t.Alias)
		if t.Privilege != nil {
			privilege := *t.Privilege
			t.Privilege = &privilege
		}
		if t.Volume != nil {
			volume := *t.Volume
			t.Volume = &volume
		}
	}
	return Session{
		Queue:    queue,
		Order:    slices.Clone(p.order),
		At:       p.at,
		Shuffle:  p.shuffle,
		Repeat:   p.repeat,
		Position: p.Position().Milliseconds(),
		Radio:    p.radio,
	}, true
}

// Restore picks up a session where it was left, paused, and loads its
// track to play at once when asked.
func (p *Player) Restore(s Session) {
	n := len(s.Queue)
	if n == 0 || len(s.Order) != n || s.At < 0 || s.At >= n {
		return
	}
	seen := make([]bool, n)
	for _, i := range s.Order {
		if i < 0 || i >= n || seen[i] {
			return
		}
		seen[i] = true
	}
	p.queue, p.order = s.Queue, s.Order
	p.shuffle, p.repeat = s.Shuffle, s.Repeat%3
	p.loadSkipping(s.At, 0, false, time.Duration(s.Position)*time.Millisecond, false)
}
