package player

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"

	"lilt/internal/qqmusic"
)

func TestPlayEmptyQueue(t *testing.T) {
	p := &Player{at: -1}
	p.PlayQueue(nil, 0) // an empty playlist's play button
	if p.Current() != nil {
		t.Error("playing from an empty queue")
	}
}

// silence is a long song of nothing.
type silence struct{}

func (silence) SampleRate() int               { return 48000 }
func (silence) Channels() int                 { return 2 }
func (silence) Len() int64                    { return 48000 * 600 }
func (silence) SetPosition(int64) error       { return nil }
func (silence) Read(b []float32) (int, error) { clear(b); return len(b), nil }

// posted collects what the player posts to the main thread, for the test
// to run there.
func posted() (chan func(), *Player) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Cue([]qqmusic.Track{{ID: 1, Name: "一", Duration: 600000}})
	out := audio.NewPlayer(silence{})
	out.SetVolume(0)
	out.Play()
	p.Attach(out)
	p.fade.attach(out, 1)
	return ch, p
}

func next(t *testing.T, ch chan func()) {
	t.Helper()
	select {
	case fn := <-ch:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("nothing posted")
	}
}

// Pausing fades out before the music stops; playing again fades it in.
func TestPauseFades(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.TogglePlay()
	if p.Playing() || !p.out.Playing() {
		t.Fatal("paused at once, or not at all")
	}
	next(t, ch)
	if p.out.Playing() {
		t.Fatal("still playing after fading out")
	}
	if !p.fade.silent() {
		t.Error("faded out to something")
	}
	p.TogglePlay()
	if !p.Playing() || !p.out.Playing() {
		t.Fatal("did not play again")
	}
	time.Sleep(fadeIn + 50*time.Millisecond)
	p.fade.mu.Lock()
	level := p.fade.level(time.Now())
	p.fade.mu.Unlock()
	if level != 1 {
		t.Errorf("faded in to %v", level)
	}
}

// Disabling playback fades makes pause and resume immediate, even during an earlier fade.
func TestPlaybackFadeDisabled(t *testing.T) {
	_, p := posted()
	defer p.out.Close()
	enabled := true
	p.PlaybackFade = func() bool { return enabled }
	p.TogglePlay()
	enabled = false
	p.TogglePlay()
	if !p.Playing() || !p.out.Playing() || p.fade.silent() {
		t.Fatal("resuming without fades did not restore playback and volume")
	}
	p.TogglePlay()
	if p.Playing() || p.out.Playing() {
		t.Fatal("pausing without fades did not stop playback immediately")
	}
	p.TogglePlay()
	if !p.Playing() || !p.out.Playing() {
		t.Fatal("resuming without fades did not start playback immediately")
	}
}

// A seek playing pauses, which fades out over a few milliseconds, and
// plays again once there, its position the one asked for meanwhile.
func TestSeekWithoutClick(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.Seek(90 * time.Second)
	if p.out.Playing() {
		t.Error("jumped while playing")
	}
	if p.Position() != 90*time.Second {
		t.Errorf("position %v while seeking", p.Position())
	}
	next(t, ch)
	if !p.out.Playing() || p.out.Position() < 90*time.Second {
		t.Errorf("after the seek: playing %v at %v", p.out.Playing(), p.out.Position())
	}
}

// Seeks one after the other, as the progress bar asks when let go, play
// on from the last.
func TestSeekTwicePlaysOn(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.Seek(90 * time.Second)
	p.Seek(95 * time.Second)
	next(t, ch)
	for len(ch) > 0 {
		next(t, ch)
	}
	time.Sleep(seekGap * 2)
	for len(ch) > 0 {
		next(t, ch)
	}
	if !p.out.Playing() || p.out.Position() < 95*time.Second {
		t.Errorf("after two seeks: playing %v at %v", p.out.Playing(), p.out.Position())
	}
}

// A radio asks for more as its queue nears its end, and adds what is not
// in it already.
func TestRadioRefills(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}})
	asked := 0
	p.AttachRadio("fm:DEFAULT", func(got func([]qqmusic.Track)) {
		asked++
		got([]qqmusic.Track{{ID: 2}, {ID: 3}})
	})
	if asked != 1 || len(p.Queue()) != 3 || p.Queue()[2].ID != 3 {
		t.Fatalf("asked %d, queue %v", asked, p.Queue())
	}
	if p.Radio() != "fm:DEFAULT" {
		t.Errorf("radio %q", p.Radio())
	}
	// Playing something else ends the radio.
	p.endRadio()
	if p.Radio() != "" || p.feed != nil {
		t.Error("radio still on")
	}
}

// Each balance mode adds its preamp to QQ's gain before peak protection, leaving unmeasured songs unchanged.
func TestNormGain(t *testing.T) {
	for _, mode := range []string{NormalizeQuiet, NormalizeStandard, NormalizeHigh} {
		for _, c := range []struct {
			st   *qqmusic.Stream
			want float64
		}{
			{nil, 1},
			{&qqmusic.Stream{}, 1},
			{&qqmusic.Stream{Gain: -7.603, Peak: 0.946}, math.Pow(10, (-7.603+normPreamp(mode))/20)},
			{&qqmusic.Stream{Gain: -6.6, Peak: 1}, min(math.Pow(10, (-6.6+normPreamp(mode))/20), 1)},
			{&qqmusic.Stream{Gain: -2, Peak: 1}, min(math.Pow(10, (-2+normPreamp(mode))/20), 1)},
			// A quiet track is lifted, but not past its peak.
			{&qqmusic.Stream{Gain: 3, Peak: 0.8}, 1.25},
			{&qqmusic.Stream{Gain: -4, Peak: 0}, math.Pow(10, (-4+normPreamp(mode))/20)},
			{&qqmusic.Stream{Gain: 3, Peak: 0}, math.Pow(10, (3+normPreamp(mode))/20)},
			{&qqmusic.Stream{Gain: 0, Peak: 0.8}, min(math.Pow(10, normPreamp(mode)/20), 1.25)},
			{&qqmusic.Stream{Gain: 0, Peak: 0.1}, math.Pow(10, normPreamp(mode)/20)},
		} {
			if g := normGain(c.st, mode); math.Abs(g-c.want) > 1e-9 {
				t.Errorf("%s %+v: %v, want %v", mode, c.st, g, c.want)
			}
		}
	}
}

// Changing normalization immediately updates audio, without compounding the gain or changing the volume choice.
func TestRefreshNormalization(t *testing.T) {
	out := audio.NewPlayer(silence{})
	defer out.Close()
	mode := NormalizeStandard
	p := &Player{volume: 0.5, info: &qqmusic.Stream{Gain: -6.6, Peak: 1}, Normalize: func() string { return mode }}
	p.fade.attach(out, 1)
	for _, balance := range []string{NormalizeStandard, NormalizeOff, NormalizeStandard, NormalizeHigh} {
		mode = balance
		p.RefreshGain()
		want := 0.25
		if balance != NormalizeOff {
			want *= min(math.Pow(10, (-6.6+normPreamp(balance))/20), 1)
		}
		if got := out.Volume(); math.Abs(got-want) > 1e-9 {
			t.Fatalf("normalization %s: output gain %v, want %v", balance, got, want)
		}
	}
	mode = NormalizeStandard
	p.SetVolume(0.8)
	if got, want := out.Volume(), 0.64*math.Pow(10, -2.6/20); math.Abs(got-want) > 1e-9 {
		t.Fatalf("volume change: output gain %v, want %v", got, want)
	}
	p.info = &qqmusic.Stream{}
	p.RefreshGain()
	if got := out.Volume(); math.Abs(got-0.64) > 1e-9 {
		t.Fatalf("unmeasured next song kept previous gain: %v", got)
	}
}

// The cache gives back what it kept, at the level asked for or a better
// one, and drops what was played longest ago once over its limit.
func TestCache(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 400)
	store := func(id int64, st *qqmusic.Stream, n int) {
		f := c.part(st)
		f.Write(make([]byte, n))
		f.Close()
		path, _ := c.keep(id, st, f.Name(), int64(n))
		c.prune(path)
	}
	store(1, &qqmusic.Stream{Level: "lossless", Type: "flac", Gain: -3}, 100)
	dl, st, ok := c.lookup(1, "exhigh")
	if !ok || dl.size != 100 || st.Type != "flac" || st.Gain != -3 {
		t.Fatalf("lookup: %v %+v", ok, st)
	}
	dl.Close()
	if _, _, ok := c.lookup(1, "hires"); ok {
		t.Error("found a lossless song for Hi-Res")
	}
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, "1-lossless.flac"), old, old)
	store(2, &qqmusic.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(3, &qqmusic.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(4, &qqmusic.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(5, &qqmusic.Stream{Level: "exhigh", Type: "mp3"}, 100)
	if _, _, ok := c.lookup(1, "standard"); ok {
		t.Error("kept the song played longest ago")
	}
	if dl, _, ok := c.lookup(5, "standard"); !ok {
		t.Error("dropped the song just kept")
	} else {
		dl.Close()
	}
	if n := c.Size(); n > 400 {
		t.Errorf("takes %d of 400", n)
	}
	c.Clear()
	if n := c.Size(); n != 0 {
		t.Errorf("cleared to %d", n)
	}
}

// Leaving a radio keeps the song playing and those before it, drops those
// it queued after, and asks it for no more.
func TestEndRadio(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})
	p.at, p.track = 1, &p.queue[1]
	asked := 0
	p.AttachRadio("heart", func(got func([]qqmusic.Track)) { asked++ })
	p.EndRadio()
	if p.Radio() != "" || p.Current().ID != 2 {
		t.Fatalf("radio %q, current %v", p.Radio(), p.Current())
	}
	if q := p.Queue(); len(q) != 2 || q[0].ID != 1 || q[1].ID != 2 {
		t.Fatalf("queue %v", q)
	}
	p.refill()
	if asked != 0 {
		t.Error("asked the radio after leaving it")
	}
}

func TestCacheCoversAndLyricsShareSpace(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 500)
	// Store a song (100B), a cover (90B), and lyrics (< 125B).
	st := &qqmusic.Stream{Level: "exhigh", Type: "mp3"}
	f := c.part(st)
	if f == nil {
		t.Fatal("no song part")
	}
	f.Write(make([]byte, 100))
	f.Close()
	songPath, ok := c.keep(1, st, f.Name(), 100)
	if !ok {
		t.Fatal("failed to keep song")
	}
	c.prune(songPath)

	coverData := bytes.Repeat([]byte("img"), 30) // 90 bytes
	if !c.KeepCover("https://example.com/cover1.jpg", coverData) {
		t.Fatal("failed to keep cover")
	}
	if !c.KeepLyrics(1, "[00:01.00]hello", "[00:01.00]你好", "") {
		t.Fatal("failed to keep lyrics")
	}

	// Reopen from disk to verify persistence and shared Size().
	reopened := NewCache(dir, 500)
	if got, ok := reopened.LookupCover("https://example.com/cover1.jpg"); !ok || !bytes.Equal(got, coverData) {
		t.Fatalf("persisted cover mismatch: ok=%v len=%d", ok, len(got))
	}
	if got, ok := reopened.LookupLyrics(1); !ok || got.LRC != "[00:01.00]hello" || got.Translation != "[00:01.00]你好" {
		t.Fatalf("persisted lyrics mismatch: ok=%v got=%+v", ok, got)
	}
	if reopened.Size() <= 190 {
		t.Fatalf("shared cache size = %d, expected song + cover + lyrics > 190", reopened.Size())
	}

	// Make the song the oldest item and tighten the shared limit so it is evicted
	// while the more recently used cover and lyrics stay.
	old := time.Now().Add(-2 * time.Hour)
	os.Chtimes(songPath, old, old)
	reopened.SetLimit(reopened.Size() - 50)
	if _, _, ok := reopened.lookup(1, "standard"); ok {
		t.Fatal("expected oldest song to be evicted when shared limit tightened")
	}
	if _, ok := reopened.LookupCover("https://example.com/cover1.jpg"); !ok {
		t.Fatal("expected newer cover to stay after evicting older song")
	}
	if _, ok := reopened.LookupLyrics(1); !ok {
		t.Fatal("expected newer lyrics to stay after evicting older song")
	}

	// Lowering limit to 0 disables persistence and drops existing entries.
	reopened.SetLimit(0)
	if reopened.Size() != 0 {
		t.Fatalf("expected size 0 after SetLimit(0), got %d", reopened.Size())
	}
	if reopened.KeepCover("https://example.com/cover2.jpg", coverData) {
		t.Fatal("kept cover while cache limit was 0")
	}
	if reopened.KeepLyrics(2, "[00:01.00]disabled", "", "") {
		t.Fatal("kept lyrics while cache limit was 0")
	}

	// Clear() removes all songs, covers and lyrics.
	reopened.SetLimit(1024)
	reopened.KeepCover("https://example.com/cover3.jpg", coverData)
	reopened.KeepLyrics(3, "[00:01.00]x", "", "")
	reopened.Clear()
	if reopened.Size() != 0 {
		t.Fatalf("expected size 0 after Clear(), got %d", reopened.Size())
	}
}

func TestPlayerLoadsCachedCoverAndLyrics(t *testing.T) {
	audioData, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	c := NewCache(dir, 10<<20)
	st := &qqmusic.Stream{Size: int64(len(audioData)), Level: "exhigh", Type: "mp3"}
	f := c.part(st)
	if f == nil {
		t.Fatal("no cache part")
	}
	if _, err := f.Write(audioData); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, ok := c.keep(42, st, f.Name(), st.Size); !ok {
		t.Fatal("not cached")
	}

	origCover, origLyrics := fetchCover, songLyrics
	t.Cleanup(func() { fetchCover, songLyrics = origCover, origLyrics })

	coverCalls, lyricCalls := 0, 0
	coverBytes := []byte("fake-jpeg-bytes")
	fetchCover = func(url string) ([]byte, error) {
		coverCalls++
		return coverBytes, nil
	}
	songLyrics = func(id int64) (string, string, string, error) {
		lyricCalls++
		return "[00:01.00]line one", "[00:01.00]第一行", "", nil
	}

	track := qqmusic.Track{
		ID:       42,
		MID:      "mid42",
		Duration: 5000,
		Album:    qqmusic.Album{PicURL: "https://y.gtimg.cn/music/photo_new/T002R500x500M000test.jpg"},
	}

	// First play populates cover and lyrics in the shared disk cache.
	ch := make(chan func(), 16)
	p1 := New(func(fn func()) { ch <- fn })
	p1.Cache = c
	p1.PlayQueue([]qqmusic.Track{track}, 0)
	deadline := time.After(2 * time.Second)
	for !(p1.Ready() && func() bool { _, loaded := p1.Lyrics(); return loaded }()) {
		select {
		case fn := <-ch:
			fn()
		case <-deadline:
			t.Fatal("timed out waiting for first playback load")
		}
	}
	p1.stop()
	if coverCalls != 1 || lyricCalls != 1 {
		t.Fatalf("first load calls: cover=%d lyrics=%d, want 1/1", coverCalls, lyricCalls)
	}
	// Wait briefly for KeepCover/KeepLyrics if posted just around Ready.
	coverURL := qqmusic.Sized(track.Album.PicURL, 600)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if _, ok := c.LookupCover(coverURL); ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Second play in a new Player with failing network functions must reuse cached cover & lyrics.
	fetchCover = func(url string) ([]byte, error) {
		coverCalls++
		return nil, errors.New("offline")
	}
	songLyrics = func(id int64) (string, string, string, error) {
		lyricCalls++
		return "", "", "", errors.New("offline")
	}
	ch2 := make(chan func(), 16)
	p2 := New(func(fn func()) { ch2 <- fn })
	p2.Cache = NewCache(dir, 10<<20)
	p2.PlayQueue([]qqmusic.Track{track}, 0)
	deadline2 := time.After(2 * time.Second)
	for !(p2.Ready() && func() bool { _, loaded := p2.Lyrics(); return loaded }()) {
		select {
		case fn := <-ch2:
			fn()
		case <-deadline2:
			t.Fatal("timed out waiting for cached playback load")
		}
	}
	p2.stop()
	if coverCalls != 1 || lyricCalls != 1 {
		t.Fatalf("offline replay hit network: cover=%d lyrics=%d", coverCalls, lyricCalls)
	}
	lines, loaded := p2.Lyrics()
	if !loaded || len(lines) != 1 || lines[0].Text != "line one" || lines[0].Translation != "第一行" {
		t.Fatalf("cached lyrics mismatch: loaded=%v lines=%+v", loaded, lines)
	}
}

func TestCachePlaylistsAndUserLists(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 64<<10)

	pl := &qqmusic.Playlist{
		ID:          777,
		Name:        "测试歌单",
		TrackCount:  2,
		Tracks:      []qqmusic.Track{{ID: 1, Name: "歌1"}, {ID: 2, Name: "歌2"}},
		Description: "desc",
	}
	if !c.KeepPlaylist(pl) {
		t.Fatal("KeepPlaylist failed")
	}
	made := []qqmusic.Playlist{{ID: 777, Name: "测试歌单", TrackCount: 2}}
	saved := []qqmusic.Playlist{{ID: 888, Name: "收藏歌单", TrackCount: 10}}
	if !c.KeepUserPlaylists(12345, made, saved) {
		t.Fatal("KeepUserPlaylists failed")
	}

	reopened := NewCache(dir, 64<<10)
	gotPl, cachedAt, ok := reopened.LookupPlaylist(777)
	if !ok || gotPl == nil || gotPl.Name != "测试歌单" || len(gotPl.Tracks) != 2 || cachedAt.IsZero() {
		t.Fatalf("LookupPlaylist mismatch: ok=%v pl=%+v cachedAt=%v", ok, gotPl, cachedAt)
	}
	gotMade, gotSaved, ok := reopened.LookupUserPlaylists(12345)
	if !ok || len(gotMade) != 1 || gotMade[0].ID != 777 || len(gotSaved) != 1 || gotSaved[0].ID != 888 {
		t.Fatalf("LookupUserPlaylists mismatch: ok=%v made=%+v saved=%+v", ok, gotMade, gotSaved)
	}
	if reopened.Size() <= 0 {
		t.Fatalf("expected non-zero cache size, got %d", reopened.Size())
	}

	// Dropping removes individual entries.
	reopened.DropPlaylist(777)
	if _, _, ok := reopened.LookupPlaylist(777); ok {
		t.Fatal("DropPlaylist did not remove playlist")
	}
	reopened.DropUserPlaylists(12345)
	if _, _, ok := reopened.LookupUserPlaylists(12345); ok {
		t.Fatal("DropUserPlaylists did not remove user playlists")
	}

	// Re-seed and verify Clear() removes playlists as well.
	if !reopened.KeepPlaylist(pl) || !reopened.KeepUserPlaylists(12345, made, saved) {
		t.Fatal("failed to re-seed playlists")
	}
	reopened.Clear()
	if reopened.Size() != 0 {
		t.Fatalf("expected size 0 after Clear(), got %d", reopened.Size())
	}
	if _, _, ok := reopened.LookupPlaylist(777); ok {
		t.Fatal("Clear() left cached playlist")
	}
	if _, _, ok := reopened.LookupUserPlaylists(12345); ok {
		t.Fatal("Clear() left cached user playlists")
	}

	// Cache limit 0 disables persistence.
	reopened.SetLimit(0)
	if reopened.KeepPlaylist(pl) || reopened.KeepUserPlaylists(12345, made, saved) {
		t.Fatal("kept playlists while cache limit was 0")
	}
}

func TestCachePurchasedSongsAndAlbums(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 64<<10)

	songs := []qqmusic.Track{{ID: 11, MID: "m11", Name: "已购单曲"}}
	albums := []qqmusic.AlbumProfile{{Album: qqmusic.Album{ID: 22, MID: "a22", Name: "已购专辑"}}}
	if !c.KeepPurchasedSongs(99, songs, true) {
		t.Fatal("KeepPurchasedSongs failed")
	}
	if !c.KeepPurchasedAlbums(99, albums, false) {
		t.Fatal("KeepPurchasedAlbums failed")
	}

	reopened := NewCache(dir, 64<<10)
	gotSongs, moreSongs, songsAt, ok := reopened.LookupPurchasedSongs(99)
	if !ok || !moreSongs || len(gotSongs) != 1 || gotSongs[0].MID != "m11" || songsAt.IsZero() {
		t.Fatalf("LookupPurchasedSongs mismatch: ok=%v more=%v songs=%+v at=%v", ok, moreSongs, gotSongs, songsAt)
	}
	gotAlbums, moreAlbums, albumsAt, ok := reopened.LookupPurchasedAlbums(99)
	if !ok || moreAlbums || len(gotAlbums) != 1 || gotAlbums[0].MID != "a22" || albumsAt.IsZero() {
		t.Fatalf("LookupPurchasedAlbums mismatch: ok=%v more=%v albums=%+v at=%v", ok, moreAlbums, gotAlbums, albumsAt)
	}
	if reopened.Size() <= 0 {
		t.Fatalf("expected non-zero cache size, got %d", reopened.Size())
	}

	reopened.DropPurchased(99)
	if _, _, _, ok := reopened.LookupPurchasedSongs(99); ok {
		t.Fatal("DropPurchased left songs")
	}
	if _, _, _, ok := reopened.LookupPurchasedAlbums(99); ok {
		t.Fatal("DropPurchased left albums")
	}

	if !reopened.KeepPurchasedSongs(99, songs, false) || !reopened.KeepPurchasedAlbums(99, albums, true) {
		t.Fatal("failed to re-seed purchased cache")
	}
	reopened.Clear()
	if reopened.Size() != 0 {
		t.Fatalf("expected size 0 after Clear(), got %d", reopened.Size())
	}

	reopened.SetLimit(0)
	if reopened.KeepPurchasedSongs(99, songs, false) || reopened.KeepPurchasedAlbums(99, albums, false) {
		t.Fatal("kept purchased cache while limit was 0")
	}
}

func TestLoadSkippingPausedAndErrorClassification(t *testing.T) {
	audioData, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	c := NewCache(dir, 10<<20)
	st := &qqmusic.Stream{Size: int64(len(audioData)), Level: "standard", Type: "mp3"}
	f := c.part(st)
	if f == nil {
		t.Fatal("no cache part")
	}
	if _, err := f.Write(audioData); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, ok := c.keep(2, st, f.Name(), st.Size); !ok {
		t.Fatal("failed to cache track 2")
	}

	origLyrics, origSongURL := songLyrics, songURL
	t.Cleanup(func() { songLyrics, songURL = origLyrics, origSongURL })
	songLyrics = func(int64) (string, string, string, error) { return "", "", "", nil }

	tracks := []qqmusic.Track{
		{ID: 1, MID: "m1", Name: "一", Duration: 100000},
		{ID: 2, MID: "m2", Name: "二", Duration: 100000},
		{ID: 3, MID: "m3", Name: "三", Duration: 100000},
	}

	drain := func(t *testing.T, ch chan func(), done func() bool) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for !done() {
			select {
			case fn := <-ch:
				fn()
			case <-deadline:
				t.Fatal("timed out waiting for player callbacks")
			}
		}
	}

	t.Run("paused Restore never skips even on ErrUnavailable or ErrLoginRequired", func(t *testing.T) {
		for _, urlErr := range []error{qqmusic.ErrUnavailable, qqmusic.ErrLoginRequired} {
			calls := 0
			songURL = func(id int64, level string) (*qqmusic.Stream, error) {
				calls++
				return nil, urlErr
			}
			ch := make(chan func(), 16)
			synced := 0
			p := New(func(fn func()) { ch <- fn })
			p.Quality = func() string { return "standard" }
			p.Changed = func() { synced++ }
			p.Restore(Session{
				Queue:    tracks,
				Order:    []int{0, 1, 2},
				At:       0,
				Position: 47000,
			})
			drain(t, ch, func() bool { return !p.Loading() && synced > 0 })
			if calls != 1 {
				t.Fatalf("paused Restore called songURL %d times, want 1", calls)
			}
			if p.Current() == nil || p.Current().ID != 1 || p.Playing() || p.Position() != 47*time.Second || p.Err() == "" {
				t.Fatalf("paused Restore altered track/position/state: cur=%v playing=%v pos=%v err=%q", p.Current(), p.Playing(), p.Position(), p.Err())
			}
		}
	})

	t.Run("active PlayQueue stops on ErrLoginRequired without traversing queue", func(t *testing.T) {
		calls := 0
		songURL = func(id int64, level string) (*qqmusic.Stream, error) {
			calls++
			return nil, qqmusic.ErrLoginRequired
		}
		ch := make(chan func(), 16)
		synced := 0
		p := New(func(fn func()) { ch <- fn })
		p.Quality = func() string { return "standard" }
		p.Changed = func() { synced++ }
		p.PlayQueue(tracks, 0)
		drain(t, ch, func() bool { return !p.Loading() && synced > 0 })
		if calls != 1 {
			t.Fatalf("PlayQueue with ErrLoginRequired called songURL %d times, want 1", calls)
		}
		if p.Current() == nil || p.Current().ID != 1 || p.Playing() || p.Err() != qqmusic.ErrLoginRequired.Error() {
			t.Fatalf("PlayQueue did not stop on ErrLoginRequired: cur=%v playing=%v err=%q", p.Current(), p.Playing(), p.Err())
		}
		beforeClear := synced
		p.ClearErr()
		if p.Err() != "" || synced != beforeClear+1 {
			t.Fatalf("ClearErr did not clear error and sync: err=%q synced=%d->%d", p.Err(), beforeClear, synced)
		}
	})

	t.Run("active PlayQueue still skips per-track ErrUnavailable to next playable track", func(t *testing.T) {
		calls := 0
		songURL = func(id int64, level string) (*qqmusic.Stream, error) {
			calls++
			return nil, qqmusic.ErrUnavailable
		}
		ch := make(chan func(), 16)
		p := New(func(fn func()) { ch <- fn })
		p.Cache = c
		p.Quality = func() string { return "standard" }
		p.PlayQueue(tracks, 0)
		drain(t, ch, func() bool { return p.Ready() })
		defer p.stop()
		if calls != 1 {
			t.Fatalf("PlayQueue skipped wrong number of unavailable tracks: calls=%d, want 1", calls)
		}
		if p.Current() == nil || p.Current().ID != 2 || !p.Playing() || p.Err() != "" {
			t.Fatalf("PlayQueue did not advance to cached track 2: cur=%v playing=%v err=%q", p.Current(), p.Playing(), p.Err())
		}
	})
}



