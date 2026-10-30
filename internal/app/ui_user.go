package app

import (
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// userDetail and userPlaylists load a user and their playlists from
// NetEase, and followedArtists and subscribedPodcasts what the listener
// follows; tests replace them.
var (
	userDetail         = qqmusic.UserDetail
	userPlaylists      = qqmusic.UserPlaylists
	followedArtists    = qqmusic.FollowedArtists
	subscribedPodcasts = qqmusic.SubscribedPodcasts
	userMembership     = qqmusic.UserMembership
)

// userData is what the app has of a user: who they are, and the playlists
// they made that anyone may see. Of the listener's own page, it has the
// artists they follow and the podcasts they subscribe to too, which
// NetEase shows no one else.
type userData struct {
	user      *qqmusic.User
	playlists []qqmusic.Playlist
	loading   int // how many of the two are on their way
	err       string

	artists       []qqmusic.ArtistProfile
	podcasts      []qqmusic.Podcast
	artistsShown  bool // whether artists has loaded once
	podcastsShown bool
	subsLoading   int
	membership    qqmusic.Membership
	memberLoading bool
	memberErr     bool
}

func (a *app) loadUser(id int64) {
	d := a.users[id]
	if d == nil {
		d = &userData{}
		a.users[id] = d
	}
	if a.profile != nil && a.profile.UserID == id {
		a.loadSubscriptions(d)
		a.loadMembership(id, d)
	}
	if d.user != nil || d.loading > 0 {
		return
	}
	d.loading, d.err = 2, ""
	go func() {
		u, err := userDetail(id)
		a.post(func() {
			d.loading--
			if err != nil {
				d.err = "用户加载失败"
				return
			}
			d.user = u
		})
	}()
	go func() {
		made, _, err := userPlaylists(id)
		a.post(func() {
			d.loading--
			if err == nil {
				d.playlists = publicPlaylists(made, id)
			}
		})
	}()
}

func (a *app) loadMembership(id int64, d *userData) {
	if d.memberLoading {
		return
	}
	d.memberLoading, d.memberErr = true, false
	fetch := userMembership
	go func() {
		level, err := fetch()
		a.post(func() { a.acceptMembership(id, d, level, err) })
	}()
}

func (a *app) acceptMembership(id int64, d *userData, level qqmusic.Membership, err error) {
	if a.profile == nil || a.profile.UserID != id || a.users[id] != d {
		return
	}
	d.memberLoading, d.memberErr = false, err != nil
	if err == nil {
		d.membership = level
	}
}

func (a *app) membershipBadge(c *ui.Context, id int64, d *userData) {
	p := a.pal
	label, ink, background := "会员状态未知", p.muted, radixSage(p.dark, 3)
	switch d.membership {
	case qqmusic.MembershipSuper:
		label, ink, background = "超级会员", radixAmber(p.dark, 11), radixAmber(p.dark, 3)
	case qqmusic.MembershipGreen:
		label, ink, background = "绿钻会员", radixGreen(p.dark, 11), radixGreen(p.dark, 3)
	case qqmusic.MembershipFree:
		label = "免费账号"
	}
	if d.memberErr {
		label, ink, background = "会员状态暂不可用", p.muted, radixSage(p.dark, 3)
	} else if d.memberLoading && d.membership == "" {
		label = "正在查询会员状态"
	}
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Box(c).Padding(3, 8).Radius(4).Background(background).Children(func() {
			ui.Text(c, label).FontSize(11).FontWeight(500).TextColor(ink).SingleLine()
		})
		if d.memberErr && ui.ButtonBase(c).Label("重试会员状态").Padding(4, 6).Children(func() {
			ui.Text(c, "重试").FontSize(12).TextColor(p.accent)
		}).Clicked() {
			a.loadMembership(id, d)
		}
	})
}

// loadSubscriptions loads the artists the listener follows and the
// podcasts they subscribe to, each time their page shows: they follow and
// subscribe on other pages, and these keep up. What they had shows while
// the lists are on their way.
func (a *app) loadSubscriptions(d *userData) {
	if d.subsLoading > 0 {
		return
	}
	d.subsLoading = 2
	go func() {
		ars, err := followedArtists()
		a.post(func() {
			d.subsLoading--
			if err == nil {
				d.artists, d.artistsShown = ars, true
			}
		})
	}()
	go func() {
		pods, err := subscribedPodcasts()
		a.post(func() {
			d.subsLoading--
			if err == nil {
				d.podcasts, d.podcastsShown = pods, true
			}
		})
	}()
}

// publicPlaylists are the playlists of lists a user made that anyone may
// see: their own, as made lists those they saved too, but not their
// private ones.
func publicPlaylists(lists []qqmusic.Playlist, uid int64) []qqmusic.Playlist {
	var out []qqmusic.Playlist
	for _, pl := range lists {
		if pl.Creator.UserID == uid && pl.Privacy != 10 {
			out = append(out, pl)
		}
	}
	return out
}

func (a *app) userPage(c *ui.Context) {
	p := a.pal
	id := a.route.id
	d := a.users[id]
	if d.user == nil {
		if d.loading == 0 {
			a.centered(c, d.err, func() { a.loadUser(id) })
			return
		}
		s := a.pageShimmer(c)
		a.scrollPage(c, func() {
			a.playlistHeaderSkeleton(c, s)
			a.playlistCardsSkeleton(c, s, a.columns(c, 168), 168, false)
		})
		return
	}
	u := d.user
	own := a.profile != nil && a.profile.UserID == id
	a.scrollPage(c, func() {
		ui.Row(c).Gap(28).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
				a.coverFill(c, u.AvatarURL, 600, artistPhotoSize/2)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Start).Gap(8).Children(func() {
				ui.Text(c, u.Nickname).MinWidth(0).FontSize(30).FontWeight(700).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
				if own {
					a.membershipBadge(c, id, d)
				}
				if u.Signature != "" {
					ui.Text(c, u.Signature).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45)
				}
			})
		})
		a.section(c, "创建的歌单")
		switch {
		case d.loading > 0:
			a.playlistCardsSkeleton(c, a.pageShimmer(c), a.columns(c, 168), 168, false)
		case len(d.playlists) == 0:
			a.empty(c, "暂无公开歌单")
		default:
			ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
				for i := range d.playlists {
					a.playlistCard(c.Key(d.playlists[i].ID), &d.playlists[i])
				}
			})
		}
		if own && d.podcastsShown {
			a.section(c, "订阅的播客")
			if len(d.podcasts) == 0 {
				a.empty(c, "还没有订阅播客")
			}
			ui.Grid(c).Columns(a.columns(c, 340)).GapX(8).GapY(4).Margin(0, -12).Children(func() {
				for i := range d.podcasts {
					a.podcastRow(c.Key(d.podcasts[i].ID), &d.podcasts[i])
				}
			})
		}
		if own && d.artistsShown {
			a.section(c, "关注的歌手")
			if len(d.artists) == 0 {
				a.empty(c, "还没有关注歌手")
			}
			ui.Grid(c).Columns(a.columns(c, 132)).GapX(20).GapY(28).Children(func() {
				for i := range d.artists {
					a.artistCard(c.Key(d.artists[i].ID), &d.artists[i])
				}
			})
		}
	})
}

// userRow is a user in a list, opening their page.
func (a *app) userRow(c *ui.Context, u *qqmusic.User) {
	p := a.pal
	row := ui.ButtonBase(c).Padding(10, 12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(u.Nickname)
	if row.Hovered() {
		row.Background(p.hover)
	}
	row.Children(func() {
		a.cover(c, u.AvatarURL, 120, 48, 24)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, u.Nickname).FontSize(14).FontWeight(600).SingleLine()
			if u.Signature != "" {
				ui.Text(c, u.Signature).FontSize(12.5).TextColor(p.muted).SingleLine()
			}
		})
	})
	if row.Clicked() {
		a.open(route{page: pageUser, id: u.UserID})
	}
}
