package app

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// wallApp is an app showing the wall of a queue of n songs, the at-th
// playing.
func wallApp(t *testing.T, n, at int) (*app, *ui.Tester) {
	a := newTestApp()
	a.settings = defaultSettings()
	var tracks []qqmusic.Track
	for i := range n {
		tracks = append(tracks, qqmusic.Track{ID: int64(i + 1), Name: fmt.Sprintf("歌%03d", i), Duration: 200000})
	}
	a.pl.Cue(tracks)
	if at > 0 {
		a.pl.Jump(at)
	}
	a.pl.SetLyrics(nil)
	a.nowPlaying, a.npPane = true, paneWall
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	return a, tt
}

// find returns where the cover of the song named s shows, failing the
// test if it is not built.
func find(t *testing.T, tt *ui.Tester, s string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("no cover of %s", s)
	}
	return r
}

// A click on a cover plays its song; a drag that starts on one moves the
// wall and plays nothing.
func TestWallPlaysTheCoverClicked(t *testing.T) {
	a, tt := wallApp(t, 300, 0)
	r := find(t, tt, "歌003")
	drag(tt, r, -300, 200*time.Millisecond)
	if got := a.pl.Current().Name; got != "歌000" {
		t.Fatalf("a drag played %s", got)
	}
	if moved := find(t, tt, "歌003"); moved.X > r.X-280 || moved.X < r.X-320 {
		t.Fatalf("dragged by 300, the cover went from %v to %v", r.X, moved.X)
	}
	if err := tt.Click("歌005"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := a.pl.Current().Name; got != "歌005" {
		t.Errorf("playing %s, want 歌005", got)
	}
}

// While the song plays, the rows sway, and a click plays the cover where
// it shows, not where it would rest: near its edge, which is far from
// where its row rests.
func TestWallPlaysTheCoverSwaying(t *testing.T) {
	a, tt := wallApp(t, 300, 0)
	a.pl.Hold(0) // playing
	for range 8 {
		tt.Advance(time.Second)
		tt.Frame()
		r := find(t, tt, "歌005")
		sway := r.X - (a.wallState.shown.X + a.wallState.pan.X + 5*wallPitch)
		if sway > -12 && sway < 12 {
			continue
		}
		x := r.X + 4 // the edge its row leads with
		if sway > 0 {
			x = r.X + r.W - 4
		}
		tt.ClickAt(x, r.Y+r.H/2)
		tt.Frame()
		if got := a.pl.Current().Name; got != "歌005" {
			t.Fatalf("clicked at its edge, swayed %v, played %s", sway, got)
		}
		return
	}
	t.Fatal("the rows never swayed")
}

// Thrown, the wall glides on past where the pointer let it go, and stops
// at its edge rather than leaving it.
func TestWallGlidesAndStops(t *testing.T) {
	a, tt := wallApp(t, 300, 0)
	ws := &a.wallState
	from := ws.shown.X
	r := find(t, tt, "歌003")
	drag(tt, r, -240, 0)
	tt.Advance(2 * time.Second)
	tt.Frame()
	if ws.shown.X > from-480 {
		t.Errorf("thrown 240 DIPs, the wall went from %v to %v", from, ws.shown.X)
	}
	// Thrown hard the other way, it stops with its first column in view.
	drag(tt, r, 3000, 0)
	tt.Advance(2 * time.Second)
	tt.Frame()
	if first := find(t, tt, "歌000"); first.X < 0 || first.X > 100 {
		t.Errorf("the first column stopped at %v", first.X)
	}
}

// Two fingers on the trackpad move the wall with them, at once.
func TestWallFollowsTheTrackpad(t *testing.T) {
	_, tt := wallApp(t, 300, 0)
	r := find(t, tt, "歌003")
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 180, 60)
	tt.Frame()
	moved := find(t, tt, "歌003")
	if dx, dy := moved.X-r.X, moved.Y-r.Y; dx > -170 || dx < -190 || dy > -50 || dy < -70 {
		t.Errorf("scrolled by (180, 60), the cover moved by (%v, %v)", dx, dy)
	}
}

// The wall opens on the song playing, however far into the queue, and
// goes to the next one played when it is out of view.
func TestWallShowsTheSongPlaying(t *testing.T) {
	a, tt := wallApp(t, 300, 250)
	in := func(s string) bool {
		r, ok := tt.Find(s)
		return ok && r.X >= 0 && r.X+r.W <= 1200 && r.Y >= 0 && r.Y+r.H <= 800
	}
	if !in("歌250") {
		t.Fatal("the wall opened away from the song playing")
	}
	a.pl.Jump(20)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	if !in("歌020") {
		t.Error("the wall stayed away from the song played next")
	}
}

// With NM_FRAMES set, saves the wall of a long queue and a short one.
func TestWallFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, n := range []int{300, 7} {
		a, tt := wallApp(t, n, min(n-1, 40))
		colors := []ui.Color{ui.RGB(220, 120, 160), ui.RGB(90, 140, 220), ui.RGB(240, 200, 110), ui.RGB(120, 200, 170), ui.RGB(170, 120, 230)}
		for i := range a.pl.Queue() {
			url := fmt.Sprintf("art%d", i)
			a.pl.Queue()[i].Album.PicURL = url
			a.art.art[qqmusic.Sized(url, 240)] = &artwork{bmp: solidBitmap(colors[i%len(colors)]), tint: colors[i%len(colors)]}
		}
		tt.Advance(time.Second)
		tt.Frame()
		save := func(name string) {
			f, _ := os.Create(filepath.Join(dir, name))
			png.Encode(f, tt.Image())
			f.Close()
		}
		save(fmt.Sprintf("wall-%d.png", n))
		// Scrolled up halfway into the frost.
		tt.Scroll(600, 600, 0, 70)
		tt.Frame()
		save(fmt.Sprintf("wall-%d-up.png", n))
	}
}

// solidBitmap is a cover of col, lighter toward its foot.
func solidBitmap(col ui.Color) *ui.Bitmap {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		c := col.Mix(ui.RGB(255, 255, 255), float32(y)/64)
		for x := range 32 {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	return ui.NewBitmap(img)
}

// coverApp is Now Playing showing the second of three songs.
func coverApp(t *testing.T) (*app, *ui.Tester, ui.Rect) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "一"}, {ID: 2, Name: "二"}, {ID: 3, Name: "三"}})
	a.pl.Next()
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 860, 560)
	// Without the tilt and the swing, which the CPU draws slowly in depth:
	// the gestures do as they would.
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tt.Advance(500 * time.Millisecond)
	r, ok := tt.Find("专辑封面")
	if !ok {
		t.Fatal("no cover")
	}
	return a, tt, r
}

// drag drags from the middle of r by dx, over a few frames, and lets go
// after hold, still.
func drag(tt *ui.Tester, r ui.Rect, dx float32, hold time.Duration) {
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	for i := 1; i <= 6; i++ {
		tt.Advance(16 * time.Millisecond)
		tt.Move(x+dx*float32(i)/6, y)
	}
	if hold > 0 {
		tt.Advance(hold)
	}
	tt.Release(x+dx, y)
	tt.Advance(time.Second)
	tt.Frame()
}

// Flung far to the left, the cover flies off for the next song; to the
// right, for the one before, however far this one has played.
func TestFlingCover(t *testing.T) {
	a, tt, r := coverApp(t)
	if a.pl.Current().ID != 2 {
		t.Fatalf("playing %d", a.pl.Current().ID)
	}
	drag(tt, r, -r.W*0.8, 0)
	if id := a.pl.Current().ID; id != 3 {
		t.Fatalf("flung left: playing %d", id)
	}
	if r2, ok := tt.Find("专辑封面"); !ok || r2.X < r.X-2 || r2.X > r.X+2 {
		t.Errorf("the next cover rests at %v, not %v", r2, r)
	}
	drag(tt, r, r.W*0.8, 0)
	if id := a.pl.Current().ID; id != 2 {
		t.Errorf("flung right: playing %d", id)
	}
}

// A drag too short, let go still, springs back, the song playing on.
func TestCoverSpringsBack(t *testing.T) {
	a, tt, r := coverApp(t)
	drag(tt, r, -r.W*0.3, 300*time.Millisecond)
	if id := a.pl.Current().ID; id != 2 {
		t.Fatalf("a short drag changed the song to %d", id)
	}
	if r2, _ := tt.Find("专辑封面"); r2.X < r.X-2 || r2.X > r.X+2 {
		t.Errorf("the cover rests at %v, not %v", r2, r)
	}
}

// Two fingers swiping across the trackpad fling it too.
func TestSwipeCover(t *testing.T) {
	a, tt, r := coverApp(t)
	x, y := r.X+r.W/2, r.Y+r.H/2
	for range 10 {
		tt.Scroll(x, y, r.W*0.06, 0)
		tt.Advance(16 * time.Millisecond)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if id := a.pl.Current().ID; id != 3 {
		t.Errorf("swiped left: playing %d", id)
	}
}

// Played from the menu or the keys, the next song's cover slides in from
// the right and the one before's from the left, each coming to rest in its
// place.
func TestCoverSlidesWithTheSong(t *testing.T) {
	a, tt, r := coverApp(t)
	tt.SetPreferences(ui.Preferences{})
	for _, step := range []struct {
		name string
		play func()
		id   int64
		dir  float32
	}{{"next", a.pl.Next, 3, 1}, {"prev", a.pl.Prev, 2, -1}} {
		step.play()
		tt.Frame()
		tt.Advance(60 * time.Millisecond)
		tt.Frame()
		if id := a.pl.Current().ID; id != step.id {
			t.Fatalf("%s: playing %d", step.name, id)
		}
		if r2, _ := tt.Find("专辑封面"); (r2.X-r.X)*step.dir < r.W*0.1 {
			t.Errorf("%s: the cover comes in at %v, from %v", step.name, r2.X, r.X)
		}
		tt.Advance(time.Second)
		tt.Frame()
		if r2, _ := tt.Find("专辑封面"); r2.X < r.X-2 || r2.X > r.X+2 {
			t.Errorf("%s: the cover rests at %v, not %v", step.name, r2, r)
		}
	}
}

// A login picks up the last stretch of listening on one device, oldest
// first: it ends where the device changes or the listener stopped for half
// an hour, and leaves out songs they may not play and songs played again.
func TestLastStretch(t *testing.T) {
	now := time.Date(2026, 10, 26, 22, 0, 0, 0, time.Local)
	play := func(id int64, ago time.Duration, device string, fee int) qqmusic.Play {
		track := qqmusic.Track{ID: id}
		if fee != 0 {
			track.Privilege = &qqmusic.Privilege{ID: id, Pl: 0}
		}
		return qqmusic.Play{Track: track, At: now.Add(-ago), Device: device}
	}
	plays := []qqmusic.Play{
		play(5, 3*time.Minute, "android", 0),
		play(4, 6*time.Minute, "android", 1), // VIP only
		play(3, 9*time.Minute, "android", 0),
		play(5, 12*time.Minute, "android", 0), // played again since
		play(2, 30*time.Minute, "android", 0),
		play(1, 70*time.Minute, "android", 0), // after a break
	}
	songs, last, ok := lastStretch(plays, now)
	var ids []int64
	for _, s := range songs {
		ids = append(ids, s.ID)
	}
	if !ok || len(ids) != 3 || ids[0] != 2 || ids[1] != 3 || ids[2] != 5 || last.Track.ID != 5 {
		t.Fatalf("picked %v, last %d", ids, last.Track.ID)
	}
	if got := heardAt(last, now); got != "3分钟前在手机上听过" {
		t.Errorf("heard %q", got)
	}

	plays[1] = play(4, 6*time.Minute, "osx", 0)
	if songs, _, _ := lastStretch(plays, now); len(songs) != 1 || songs[0].ID != 5 {
		t.Errorf("crossed devices: %v", songs)
	}
	plays[0] = play(5, 4, "android", 1)
	if songs, last, _ := lastStretch(plays[:1], now); len(songs) != 0 || last.Track.ID != 0 {
		t.Errorf("picked a song that may not play: %v", songs)
	}
	if _, _, ok := lastStretch([]qqmusic.Play{play(1, 8*24*time.Hour, "iphone", 0)}, now); ok {
		t.Error("picked up a week-old song")
	}
}
