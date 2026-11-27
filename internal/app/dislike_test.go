package app

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

type blacklistFixtureReply struct {
	list qqmusic.SongBlacklist
	err  error
}

type blacklistFixtureRead struct {
	uid   int64
	reply chan blacklistFixtureReply
}

func stubSongBlacklist(t *testing.T) chan blacklistFixtureRead {
	t.Helper()
	reads, stop := make(chan blacklistFixtureRead, 8), make(chan struct{})
	old := readSongBlacklist
	readSongBlacklist = func(uid int64) (qqmusic.SongBlacklist, error) {
		reply := make(chan blacklistFixtureReply, 1)
		reads <- blacklistFixtureRead{uid, reply}
		select {
		case r := <-reply:
			return r.list, r.err
		case <-stop:
			return qqmusic.SongBlacklist{}, nil
		}
	}
	t.Cleanup(func() { readSongBlacklist = old; close(stop) })
	return reads
}

func TestDailyBlacklistSyncOnlyAppliesToDailyRecommendationTracks(t *testing.T) {
	reads, posted := stubSongBlacklist(t), make(chan func(), 8)
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 10001}
	a.postFn = func(fn func()) { posted <- fn }
	daily := qqmusic.Track{ID: 42, Name: "QQ 音乐屏蔽曲目", Duration: 200000, RecommendationAccount: 10001, FromDailyRecommendations: true}
	fm, ordinary := daily, daily
	fm.FromDailyRecommendations = false
	ordinary.FromDailyRecommendations, ordinary.RecommendationAccount = false, 0
	a.daily = []qqmusic.Track{daily, {ID: 43, Name: "下一首", RecommendationAccount: 10001, FromDailyRecommendations: true}}
	a.route = route{page: pageDaily}
	a.pl.Cue(a.daily)
	a.pl.Hold(12 * time.Second)
	a.load(a.route)
	read := wait(t, reads, "the daily blacklist request")
	a.load(a.route) // In flight; revisiting the page shares the same request.
	if read.uid != a.profile.UserID || len(reads) != 0 {
		t.Fatal("daily blacklist should read once for the active account")
	}
	read.reply <- blacklistFixtureReply{list: qqmusic.SongBlacklist{IDs: map[int64]struct{}{42: {}}, Complete: true}}
	wait(t, posted, "daily blacklist result")()
	if a.pl.Current().ID != 42 || a.pl.Position() != 12*time.Second || !a.pl.Playing() {
		t.Fatal("read-only sync should never interrupt current playback")
	}
	if a.firstDailyPlayable(a.daily) != 1 || !a.skipRecommendation(&daily) || a.skipRecommendation(&fm) ||
		a.dislikeState(&ordinary) != 0 || a.blocksNewLike(&fm) {
		t.Fatal("the same song should be skipped only in the daily list")
	}
	tt := ui.NewTester(a.view, 1180, 780)
	if row, ok := tt.AccessByRole(ui.RoleButton, "QQ 音乐屏蔽曲目（已不喜欢）"); !ok || row.Disabled {
		t.Fatal("the synced match should stay visible and selectable")
	}
	// Now Playing shares the layout, but the remote mark is only for daily sources.
	a.nowPlaying = true
	a.comments = commentState{track: daily.ID, loaded: true}
	tt.Advance(time.Second)
	tt.Frame()
	if button, ok := tt.AccessByRole(ui.RoleButton, "已不喜欢"); !ok || !button.Disabled {
		t.Fatal("daily playback did not show the synced state")
	}
	a.pl.Cue([]qqmusic.Track{fm})
	tt.Frame()
	if button, ok := tt.AccessByRole(ui.RoleButton, "不喜欢这首歌"); !ok || button.Disabled {
		t.Fatal("the same song in FM should not receive the synced state")
	}

	for _, step := range []struct {
		name  string
		reply blacklistFixtureReply
		want  bool
	}{
		{"failed refresh", blacklistFixtureReply{err: errors.New("offline")}, true},
		{"incomplete refresh", blacklistFixtureReply{list: qqmusic.SongBlacklist{IDs: map[int64]struct{}{44: {}}}}, true},
		{"complete refresh after removing in QQ Music", blacklistFixtureReply{list: qqmusic.SongBlacklist{Complete: true}}, false},
	} {
		a.load(a.route)
		wait(t, reads, step.name).reply <- step.reply
		wait(t, posted, step.name)()
		if got := a.skipRecommendation(&daily); got != step.want {
			t.Fatalf("%s: skipped=%t, want %t", step.name, got, step.want)
		}
	}
}

func TestDailyBlacklistSyncRejectsStaleAccountAndPreservesConfirmedClick(t *testing.T) {
	reads, posted := stubSongBlacklist(t), make(chan func(), 8)
	old := dislikeSong
	dislikeSong = func(int64, int64) error { return nil }
	t.Cleanup(func() { dislikeSong = old })

	a := newTestApp()
	a.profile = &qqmusic.Profile{UserID: 10001}
	a.postFn = func(fn func()) { posted <- fn }
	a.loadDailyDislikes()
	previous := wait(t, reads, "prior login's blacklist request")

	a.profile = &qqmusic.Profile{UserID: 10001} // Signing back in also invalidates old results.
	a.loadDailyDislikes()
	current := wait(t, reads, "new login's blacklist request")
	previous.reply <- blacklistFixtureReply{list: qqmusic.SongBlacklist{IDs: map[int64]struct{}{43: {}}, Complete: true}}
	wait(t, posted, "prior login's blacklist reply")()
	if len(a.dailyDislikes.ids) != 0 || !a.dailyDislikes.loading {
		t.Fatal("prior login's result leaked into the new session")
	}

	a.nowPlaying = true
	a.pl.Cue([]qqmusic.Track{{ID: 42, RecommendationAccount: 10001, FromDailyRecommendations: true}})
	a.dislikeCurrent(a.pl.Current())
	wait(t, posted, "explicit dislike completed while the read is pending")()
	current.reply <- blacklistFixtureReply{list: qqmusic.SongBlacklist{Complete: true}}
	wait(t, posted, "blacklist snapshot from before the explicit dislike")()
	if !a.skipRecommendation(a.pl.Current()) {
		t.Fatal("a concurrent read erased an explicit dislike that just succeeded")
	}
}

func TestDislikeAppearsOnlyBesideNowPlayingHeart(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 10001}
	tr := qqmusic.Track{ID: 42, Name: "推荐歌曲", RecommendationAccount: 10001, Duration: 200000}
	a.daily = []qqmusic.Track{tr}
	a.pl.Cue(a.daily)
	a.comments = commentState{track: tr.ID, loaded: true}
	tt := ui.NewTester(a.view, 880, 600)
	const label = "不喜欢这首歌"
	for _, page := range []pageKind{pageDaily, pageFM} {
		a.route = route{page: page}
		tt.Frame()
		if _, ok := tt.AccessByRole(ui.RoleButton, label); ok {
			t.Fatal("dislike appeared outside expanded Now Playing")
		}
	}
	a.nowPlaying = true
	tt.Advance(time.Second)
	for _, pane := range []int{paneLyrics, paneStage, paneWall} {
		a.npPane = pane
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		heart, hasHeart := tt.AccessByRole(ui.RoleButton, "喜欢")
		dislike, hasDislike := tt.AccessByRole(ui.RoleButton, label)
		if !hasHeart || !hasDislike || dislike.Disabled || dislike.Bounds.X < heart.Bounds.X+heart.Bounds.W-1 || dislike.Bounds.W < 20 {
			t.Fatalf("pane %d: heart at %v, dislike at %v", pane, heart.Bounds, dislike.Bounds)
		}
		count := 0
		for _, item := range tt.AccessAll() {
			if item.Role == ui.RoleButton && item.Name == label {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("pane %d: %d dislike buttons", pane, count)
		}
		saveTransitionFrame(t, tt, fmt.Sprintf("dislike-%d", pane))
	}
	tests := []struct {
		name    string
		profile *qqmusic.Profile
		track   qqmusic.Track
	}{
		{"same song from ordinary playback", a.profile, qqmusic.Track{ID: tr.ID, Name: tr.Name}},
		{"different account", &qqmusic.Profile{UserID: 10002}, tr},
		{"anonymous", nil, tr},
	}
	for _, tc := range tests {
		a.profile = tc.profile
		a.pl.Cue([]qqmusic.Track{tc.track})
		tt.Frame()
		tt.Frame()
		if _, ok := tt.AccessByRole(ui.RoleButton, label); ok {
			t.Fatalf("dislike appeared for %s", tc.name)
		}
	}
}

func TestDislikeNowPlayingAdvancesOnlyAfterConfirmation(t *testing.T) {
	type request struct{ uid, song int64 }
	calls := make(chan request, 4)
	finish := make(chan error, 1)
	post := make(chan func(), 4)
	old := dislikeSong
	dislikeSong = func(uid, id int64) error {
		calls <- request{uid, id}
		return <-finish
	}
	t.Cleanup(func() { dislikeSong = old; close(finish) })
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 10001}
	a.postFn = func(fn func()) { post <- fn }
	a.pl.Cue([]qqmusic.Track{{ID: 42, Name: "推荐歌曲", RecommendationAccount: 10001, Duration: 200000}})
	a.comments = commentState{track: 42, loaded: true}
	a.pl.Hold(20 * time.Second)
	var radioBatch func([]qqmusic.Track)
	a.pl.AttachRadio(fmRadio(qqmusic.FMDefault), func(got func([]qqmusic.Track)) { radioBatch = got })
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("不喜欢这首歌"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, calls, "the dislike request"); got != (request{10001, 42}) {
		t.Fatalf("request: %+v", got)
	}
	tt.Frame()
	pending, ok := tt.AccessByRole(ui.RoleButton, "正在提交不喜欢")
	if !ok || !pending.Disabled {
		t.Fatal("dislike was still enabled while waiting for QQ Music")
	}
	tt.Move(10, 10)
	tt.Advance(time.Second)
	saveTransitionFrame(t, tt, "dislike-pending")
	a.dislikeCurrent(a.pl.Current())
	finish <- qqmusic.ErrDislikeUnconfirmed
	wait(t, post, "the unconfirmed result")()
	if a.toast != qqmusic.ErrDislikeUnconfirmed.Error() || a.dislikes.songs[42] != 0 || a.pl.Current().ID != 42 || a.pl.Position() != 20*time.Second || a.pl.Loading() {
		t.Fatal("unconfirmed feedback did not leave playback alone")
	}
	tt.Frame()
	if err := tt.Click("不喜欢这首歌"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, calls, "the next explicit dislike request"); got != (request{10001, 42}) {
		t.Fatalf("request: %+v", got)
	}
	finish <- nil
	wait(t, post, "the confirmed result")()
	if a.toast != "" || !a.pl.Loading() || a.liked[42] {
		t.Fatal("confirmed feedback should wait for the next radio song without a toast or favorite")
	}
	radioBatch(nil)
	a.pl.EndRadio()
	tt.Frame()
	confirmed, ok := tt.AccessByRole(ui.RoleButton, "已不喜欢")
	if !ok || !confirmed.Disabled || confirmed.Focusable {
		t.Fatal("confirmed feedback should be announced as a disabled button")
	}
	tt.ClickAt(confirmed.Bounds.X+confirmed.Bounds.W/2, confirmed.Bounds.Y+confirmed.Bounds.H/2)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(4 * time.Second)
	if len(calls) != 0 || a.toast != "" || a.pl.Current().ID != 42 {
		t.Fatal("interacting with a confirmed dislike must do nothing")
	}
	saveTransitionFrame(t, tt, "dislike-confirmed")
	a.pl.Cue([]qqmusic.Track{{ID: 43, RecommendationAccount: 10001}})
	a.dislikeCurrent(a.pl.Current())
	if got := wait(t, calls, "the request before manually changing tracks"); got != (request{10001, 43}) {
		t.Fatalf("request: %+v", got)
	}
	a.pl.Cue([]qqmusic.Track{{ID: 44, RecommendationAccount: 10001}})
	a.pl.Hold(5 * time.Second)
	finish <- nil
	wait(t, post, "the reply after manually changing tracks")()
	if a.toast != "" || a.pl.Current().ID != 44 || !a.pl.Playing() || a.pl.Position() != 5*time.Second || a.dislikes.songs[43] != dislikeConfirmed {
		t.Fatal("feedback advanced again after manually changing tracks")
	}
	// Even signing back into the same account replaces the active profile.
	// A completion from the earlier login must not change its view.
	a.dislikeCurrent(a.pl.Current())
	if got := wait(t, calls, "the request before changing profiles"); got != (request{10001, 44}) {
		t.Fatalf("request: %+v", got)
	}
	a.profile = &qqmusic.Profile{UserID: 10001}
	finish <- nil
	wait(t, post, "the reply for the old profile")()
	if a.toast != "" || a.dislikes.songs[44] == dislikeConfirmed || !a.pl.Playing() {
		t.Fatal("the old profile's reply changed the current view")
	}
}

func TestDislikedRecommendationStaysVisibleAndCanBeSelected(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 10001}
	a.dislikes = &songDislikes{a.profile, map[int64]dislikeStatus{42: dislikeConfirmed}}
	a.daily = []qqmusic.Track{
		{ID: 42, Name: "已屏蔽的歌曲", RecommendationAccount: 10001, Duration: 200000},
		{ID: 43, Name: "未屏蔽的歌曲", RecommendationAccount: 10001, Duration: 200000},
	}
	a.route = route{page: pageDaily}
	tt := ui.NewTester(a.view, 1180, 780)
	for _, label := range []string{"播放全部", "随机播放"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		if a.pl.Current().ID != 43 || len(a.pl.Queue()) != 2 {
			t.Fatalf("%s should start on the unblocked song and retain both rows", label)
		}
	}
	tt.Frame()
	const skippedRow = "已屏蔽的歌曲（已不喜欢）"
	if row, ok := tt.AccessByRole(ui.RoleButton, skippedRow); !ok || row.Disabled {
		t.Fatal("the daily list did not mark the disliked song")
	}
	saveTransitionFrame(t, tt, "dislike-daily-list")
	if err := tt.Click(skippedRow); err != nil {
		t.Fatal(err)
	}
	if a.pl.Current().ID != 42 || !a.pl.Playing() || !a.skipRecommendation(a.pl.Current()) {
		t.Fatal("selecting the disliked daily row should play it without clearing feedback")
	}
	a.nowPlaying = true
	a.comments = commentState{track: 42, loaded: true}
	tt.Advance(time.Second)
	tt.Frame()
	if button, ok := tt.AccessByRole(ui.RoleButton, "已不喜欢"); !ok || !button.Disabled {
		t.Fatal("replaying the disliked song should still show the confirmed status")
	}
	saveTransitionFrame(t, tt, "dislike-replay")

	a.pl.Cue(a.daily[1:])
	a.pl.AttachRadio(fmRadio(qqmusic.FMDefault), func(got func([]qqmusic.Track)) { got(a.daily[:1]) })
	queue := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(radixSage(true, 2)).Children(func() { a.queuePane(c) })
	}, 550, 400)
	queue.Frame()
	if row, ok := queue.AccessByRole(ui.RoleButton, skippedRow); !ok || row.Disabled {
		t.Fatal("the radio queue did not mark the disliked song")
	}
	saveTransitionFrame(t, queue, "dislike-radio-queue")
	if err := queue.Click(skippedRow); err != nil {
		t.Fatal(err)
	}
	if a.pl.Current().ID != 42 || !a.pl.Playing() || a.pl.Radio() == "" {
		t.Fatal("selecting the disliked FM row should not leave the radio")
	}
}

func TestDislikedSongDoesNotAcceptNewLike(t *testing.T) {
	a := newTestApp()
	a.profile = &qqmusic.Profile{UserID: 10001}
	a.dislikes = &songDislikes{a.profile, map[int64]dislikeStatus{42: dislikeConfirmed}}
	tr := qqmusic.Track{ID: 42, Name: "歌曲"}
	a.pl.Cue([]qqmusic.Track{tr})
	tt := ui.NewTester(func(c *ui.Context) { a.likeButton(c, &tr, 20, white) }, 200, 120)
	heart, ok := tt.AccessByRole(ui.RoleButton, "喜欢")
	if !ok || !heart.Disabled {
		t.Fatal("the heart should be disabled for an already blocked song, including in ordinary playback")
	}
	a.likeCurrent()
	if a.isLiked(&tr) || a.toast != "请先在 QQ 音乐黑名单中解除屏蔽" {
		t.Fatal("the like shortcut should direct the listener to the existing blacklist")
	}
	a.liked = map[int64]bool{42: true}
	tt.Frame()
	heart, ok = tt.AccessByRole(ui.RoleButton, "取消喜欢")
	if !ok || heart.Disabled || !a.isLiked(&tr) {
		t.Fatal("a pre-existing like must remain available to remove")
	}
}
