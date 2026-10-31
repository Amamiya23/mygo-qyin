package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// likes are the songs and episodes the listener likes, which hearts show
// filled.

// These like or unlike a song or an episode with NetEase, and ask which
// episodes the listener likes; tests replace them.
var (
	likeSong        = qqmusic.Like
	likeProgram     = qqmusic.LikeProgram
	programsLiked   = qqmusic.ProgramsLiked
	readLikedAlbums = qqmusic.LikedAlbums
	readAlbumLiked  = qqmusic.AlbumLiked
	likeAlbum       = qqmusic.LikeAlbum
)

const likedAlbumsLimit = 30

type likedAlbumData struct {
	albums []qqmusic.AlbumProfile
	page   paged
}

// Each collection keeps its own place when switching tabs or returning from a detail page.
type likedScrollState struct {
	songs  listScrollState
	albums ui.ListState
}

func (a *app) isLikedPlaylist(id int64) bool {
	return id > 0 && a.likedPlaylist() == id
}

func (a *app) loadLikedAlbums(again bool) {
	if a.profile == nil || a.likedAlbums != nil && !again {
		return
	}
	d := &likedAlbumData{}
	a.likedAlbums = d
	a.moreLikedAlbums(d)
}

func (a *app) moreLikedAlbums(d *likedAlbumData) {
	if a.profile == nil || d.page.loading {
		return
	}
	uid, offset := a.profile.UserID, len(d.albums)
	d.page.loading, d.page.err = true, ""
	go func() {
		albums, more, err := readLikedAlbums(likedAlbumsLimit, offset)
		a.post(func() {
			if a.profile == nil || a.profile.UserID != uid || a.likedAlbums != d {
				return
			}
			d.page.loading = false
			if err != nil {
				d.page.err = "收藏专辑加载失败"
				return
			}
			d.albums = append(d.albums, albums...)
			d.page.loaded, d.page.more = true, more && len(albums) > 0
		})
	}()
}

func (a *app) likedPlaylistPage(c *ui.Context, id int64) {
	pl := a.playlists[id]
	content := route{page: pagePlaylist, id: id}
	header := func(c *ui.Context) {
		a.likedCollectionHeader(c, id)
		ui.Row(c).AlignItems(ui.Center).Gap(12).Margin(0, 0, 18, 0).Children(func() {
			tab := a.route.tab
			if segmented(c.Key("liked-kind"), &tab, "单曲", "专辑") {
				r := a.route
				r.tab = tab
				a.show(r)
			}
		})
	}
	if a.route.tab == 1 {
		d := a.likedAlbums
		if d == nil {
			a.loadLikedAlbums(false)
			d = a.likedAlbums
		}
		a.listPageWithKey(c, content, &a.likedScroll.albums, func() {
			header(c)
			if d == nil {
				return
			}
			a.albumGrid(c, d.albums)
			if !d.page.loaded && d.page.loading {
				a.playlistCardsSkeleton(c, a.pageShimmer(c), a.columns(c, 156), 156, false)
				return
			}
			a.pageFoot(c, &d.page, func() { a.moreLikedAlbums(d) })
			if d.page.loaded && len(d.albums) == 0 {
				a.empty(c, "暂无收藏专辑")
			}
		})
		return
	}
	if pl == nil {
		if a.loadingPl[id] {
			a.listPageWithKey(c, content, &a.likedScroll.songs.ListState, func() {
				a.underTop, a.underBottom = false, true
				a.trackSkeletonRows(c, a.pageShimmer(c), true, header)
			})
		} else {
			a.listPageWithKey(c, content, &a.likedScroll.songs.ListState, func() {
				header(c)
				a.centered(c, a.plErr[id], func() { a.loadPlaylist(id, false) })
			})
		}
		return
	}
	a.trackRowsWithKey(c, content, &a.likedScroll.songs, pl.Tracks, true, header)
}

func (a *app) likedCollectionHeader(c *ui.Context, id int64) {
	p := a.pal
	pl := a.playlists[id]
	loaded := pl != nil
	if pl == nil {
		for i := range a.mine {
			if a.mine[i].ID == id {
				pl = &a.mine[i]
				break
			}
		}
	}
	if pl == nil {
		pl = &qqmusic.Playlist{ID: id, Name: "我喜欢"}
	}
	count := pl.TrackCount
	if loaded {
		count = len(pl.Tracks)
	}
	meta := fmt.Sprintf("%d 首单曲", count)
	if a.route.tab == 1 {
		meta = "正在加载专辑"
		if d := a.likedAlbums; d != nil {
			switch {
			case d.page.loaded:
				more := ""
				if d.page.more {
					more = "+"
				}
				meta = fmt.Sprintf("%d%s 张专辑", len(d.albums), more)
			case d.page.err != "":
				meta = "专辑暂不可用"
			}
		}
	}
	a.collectionHeader(c, pl, "收藏", meta, func() {
		if a.route.tab == 1 {
			d := a.likedAlbums
			pillButton(c.Key("refresh-liked-albums"), iconReload, "刷新", p.hover.Over(p.bg), p.text).
				Label("刷新收藏专辑").Disabled(d == nil || d.page.loading).
				OnClick(func() { a.loadLikedAlbums(true) })
			return
		}
		pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Disabled(!loaded || len(pl.Tracks) == 0).
			OnClick(func() {
				if a.pl.Shuffle() {
					a.pl.ToggleShuffle()
				}
				a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks))
			})
		pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Disabled(!loaded || len(pl.Tracks) == 0).
			OnClick(func() {
				if !a.pl.Shuffle() {
					a.pl.ToggleShuffle()
				}
				a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks))
			})
	})
}

func (a *app) loadAlbumLike(d *albumData) {
	if a.profile == nil || d.album == nil || d.album.MID == "" || d.likeUID == a.profile.UserID && (d.likeLoading || d.likeKnown) {
		return
	}
	uid, mid := a.profile.UserID, d.album.MID
	d.likeUID, d.likeKnown, d.likeLoading = uid, false, true
	go func() {
		liked, err := readAlbumLiked(mid)
		a.post(func() { a.albumLikeStatusAnswered(uid, d, liked, err) })
	}()
}

func (a *app) albumLikeStatusAnswered(uid int64, d *albumData, liked bool, err error) {
	if a.profile == nil || a.profile.UserID != uid || d.likeUID != uid {
		return
	}
	d.likeLoading = false
	if err == nil {
		d.liked, d.likeKnown = liked, true
	}
}

func (a *app) albumLikeButton(c *ui.Context, al *qqmusic.AlbumProfile) {
	d := a.albums[al.ID]
	if d == nil || al.MID == "" {
		return
	}
	if a.profile != nil && (d.likeUID != a.profile.UserID || !d.likeKnown) {
		label := "重试专辑收藏状态"
		if d.likeLoading {
			label = "正在读取专辑收藏状态"
		}
		iconButton(c.Key("like-album"), iconReload, 20, a.pal.muted, label).Disabled(d.likeLoading).
			OnClick(func() { a.loadAlbumLike(d) })
		return
	}
	icon, label, color := iconHeart, "收藏专辑", a.pal.muted
	if a.profile != nil && d.likeUID == a.profile.UserID && d.likeKnown && d.liked {
		icon, label, color = iconHeartFill, "取消收藏专辑", radixRed(a.pal.dark, 9)
	}
	iconButton(c.Key("like-album"), icon, 20, color, label).Disabled(d.likeLoading).
		OnClick(func() { a.toggleAlbumLike(d) })
}

func (a *app) toggleAlbumLike(d *albumData) {
	if a.profile == nil {
		a.openLogin()
		return
	}
	if d.album == nil || d.album.MID == "" || d.likeLoading {
		return
	}
	if d.likeUID != a.profile.UserID || !d.likeKnown {
		a.loadAlbumLike(d)
		return
	}
	uid, mid := a.profile.UserID, d.album.MID
	liked := !(d.likeUID == uid && d.likeKnown && d.liked)
	d.likeUID, d.likeLoading = uid, true
	go func() {
		err := likeAlbum(mid, liked)
		a.post(func() { a.albumLikeAnswered(uid, d, liked, err) })
	}()
}

func (a *app) albumLikeAnswered(uid int64, d *albumData, liked bool, err error) {
	if a.profile == nil || a.profile.UserID != uid || d.likeUID != uid {
		return
	}
	d.likeLoading = false
	if err != nil {
		a.toast = "专辑收藏操作失败，请稍后再试"
		return
	}
	d.liked, d.likeKnown = liked, true
	// Re-read the collection next time; its ordering belongs to QQ Music.
	a.likedAlbums = nil
	if a.route.page == pagePlaylist && a.route.tab == 1 && a.isLikedPlaylist(a.route.id) {
		a.loadLikedAlbums(false)
	}
}

// loadLiked loads the listener's liked songs.
func (a *app) loadLiked(uid int64) {
	go func() {
		ids, err := qqmusic.LikedIDs(uid)
		a.post(func() {
			if err != nil || a.profile == nil || a.profile.UserID != uid {
				return
			}
			a.liked = make(map[int64]bool, len(ids))
			for _, id := range ids {
				a.liked[id] = true
			}
		})
	}()
}

// toggleLike likes a song or an episode, or unlikes it, at once in the app
// and then with NetEase, which may refuse: the heart goes back then. Signed
// out, it asks the listener to sign in.
func (a *app) toggleLike(tr *qqmusic.Track) {
	if a.profile == nil {
		a.showMain()
		a.openLogin()
		return
	}
	if tr.ProgramID != 0 {
		a.toggleProgramLike(tr.ProgramID)
		return
	}
	if a.liked == nil {
		a.liked = map[int64]bool{}
	}
	id, like := tr.ID, !a.liked[tr.ID]
	a.setLiked(id, like)
	go func() {
		err := likeSong(id, like)
		a.post(func() {
			if err != nil {
				a.setLiked(id, !like)
				a.toast = "操作失败，请稍后再试"
				return
			}
			// The liked songs' playlist changed: load it anew when shown.
			for _, pl := range a.mine {
				if pl.SpecialType == 5 {
					delete(a.playlists, pl.ID)
					if a.route.page == pagePlaylist && a.route.id == pl.ID {
						a.loadPlaylist(pl.ID, false)
					}
				}
			}
		})
	}()
}

func (a *app) toggleProgramLike(id int64) {
	like := !a.likedPrograms[id]
	a.setProgramLiked(id, like)
	go func() {
		err := likeProgram(id, like)
		a.post(func() {
			if err != nil {
				a.setProgramLiked(id, !like)
				a.toast = "操作失败，请稍后再试"
			}
		})
	}()
}

// isLiked reports whether the listener likes tr, a song or an episode.
func (a *app) isLiked(tr *qqmusic.Track) bool {
	if tr.ProgramID != 0 {
		return a.likedPrograms[tr.ProgramID]
	}
	return a.liked[tr.ID]
}

// likeCurrent likes the song or episode playing, or unlikes it, from the
// keyboard (⌘D), which shows no heart: a toast says which it did.
func (a *app) likeCurrent() {
	tr := a.pl.Current()
	if tr == nil {
		return
	}
	a.toggleLike(tr)
	switch {
	case a.profile == nil:
		return // asked to sign in
	case !a.isLiked(tr):
		a.toast = "已取消喜欢"
	case tr.ProgramID != 0:
		a.toast = "已喜欢这期节目"
	default:
		a.toast = "已添加到我喜欢的音乐"
	}
}

func (a *app) setLiked(id int64, like bool) {
	if like {
		a.liked[id] = true
	} else {
		delete(a.liked, id)
	}
}

// setProgramLiked says the listener likes an episode, or doesn't, over
// what NetEase may still answer of it.
func (a *app) setProgramLiked(id int64, like bool) {
	if a.likedPrograms == nil {
		a.likedPrograms = map[int64]bool{}
	}
	a.likedPrograms[id] = like
	delete(a.askedPrograms, id)
}

// askProgramLikes asks NetEase whether the listener likes the episodes
// ids, those not asked about yet: NetEase lists no episodes they like.
// An episode it doesn't answer of shows not liked, and isn't asked again.
func (a *app) askProgramLikes(ids []int64) {
	if a.profile == nil {
		return
	}
	var ask []int64
	for _, id := range ids {
		if _, known := a.likedPrograms[id]; !known && !a.askedPrograms[id] {
			ask = append(ask, id)
		}
	}
	if len(ask) == 0 {
		return
	}
	if a.askedPrograms == nil {
		a.askedPrograms = map[int64]bool{}
	}
	for _, id := range ask {
		a.askedPrograms[id] = true
	}
	uid := a.profile.UserID
	go func() {
		liked, _ := programsLiked(ask)
		a.post(func() { a.programLikesAnswered(uid, ask, liked) })
	}()
}

// programLikesAnswered takes NetEase's answer of whether uid likes the
// episodes asked about.
func (a *app) programLikesAnswered(uid int64, asked []int64, liked map[int64]bool) {
	if a.profile == nil || a.profile.UserID != uid {
		return
	}
	for _, id := range asked {
		// A heart clicked meanwhile knows better.
		if a.askedPrograms[id] {
			a.setProgramLiked(id, liked[id])
		}
	}
}

// likeButton is a heart that likes tr: filled in red once liked. Liking
// pops it in on a spring while a ring swells around it and confetti flies
// out; unliking lets it give a little.
func (a *app) likeButton(c *ui.Context, tr *qqmusic.Track, size float32, color ui.Color, surface ...buttonSurface) ui.Element {
	if tr.ProgramID != 0 {
		a.askProgramLikes([]int64{tr.ProgramID})
	}
	liked := a.isLiked(tr)
	icon, label := iconHeart, "喜欢"
	if liked {
		icon, color, label = iconHeartFill, radixRed(a.pal.dark, 9), "取消喜欢"
	}
	// The burst goes on a wrapper, which the heart's pop does not scale.
	wrap := ui.Box(c).Center()
	var b ui.Element
	wrap.Children(func() { b = iconButton(c, icon, size, color, label, surface...) })
	scale := b.AnimateTo("pop", 1, ui.Animation{Spring: ui.SpringOptions{Duration: 550 * time.Millisecond, Bounce: 0.55}})
	b.Scale(scale, scale)
	burst := ui.Local(wrap, "burst", func() time.Time { return time.Time{} })
	if b.Clicked() {
		switch {
		case a.profile == nil:
		case liked:
			b.Jump("pop", 0.8)
		default:
			b.Jump("pop", 0.2)
			*burst = c.Now()
		}
		a.toggleLike(tr)
	}
	if !burst.IsZero() {
		start, accent := *burst, radixRed(a.pal.dark, 9)
		wrap.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if drawBurst(p, r, size, p.Now().Sub(start), accent) {
				p.AnimationFrame()
			}
		})
	}
	return wrap
}

// burstColors are the confetti's colors, around the heart's red.
var burstColors = []ui.Color{
	radixRed(false, 9), radixOrange9, radixAmber(false, 9),
	radixPurple9, radixCyan9, radixPink9,
}

const burstTime = 750 * time.Millisecond

// drawBurst draws the burst of a heart of size in r, at elapsed since the
// like, and reports whether it goes on.
func drawBurst(p *ui.Painter, r ui.Rect, size float32, elapsed time.Duration, accent ui.Color) bool {
	t := float32(elapsed) / float32(burstTime)
	if t >= 1 || t < 0 {
		return false
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	out := func(x float32) float32 { return 1 - (1-x)*(1-x)*(1-x) } // ease out

	// The ring swells from the heart and thins away, in the first half.
	if rt := t * 2; rt < 1 {
		radius := size * (0.35 + 0.65*out(rt))
		width := size * 0.22 * (1 - rt)
		ring := (&ui.Path{}).Circle(cx, cy, radius)
		p.StrokePath(ring, width, accent.Alpha(0.9*(1-rt)))
	}

	// Two rings of confetti fly out, slowing, shrinking and fading; the
	// inner one a little behind and turned, as if thrown at once.
	for ring := range 2 {
		lag := float32(ring) * 0.08
		pt := (t - 0.12 - lag) / (0.88 - lag)
		if pt <= 0 {
			continue
		}
		pt = min(pt, 1)
		const n = 7
		dist := size * (0.55 + (1.15-0.25*float32(ring))*out(pt))
		dot := size * (0.16 - 0.04*float32(ring)) * (1 - pt*pt)
		alpha := 1 - pt*pt
		for i := range n {
			angle := (float64(i)/n + float64(ring)*0.5/n - 0.25) * 2 * math.Pi
			x := cx + dist*float32(math.Cos(angle))
			y := cy + dist*float32(math.Sin(angle))
			color := burstColors[(i+ring*3)%len(burstColors)].Alpha(alpha)
			p.Fill(ui.Rect{X: x - dot, Y: y - dot, W: 2 * dot, H: 2 * dot}, color, dot)
		}
	}
	return true
}
