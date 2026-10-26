package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// artistLinks shows a track's artists as its credits read, each a link to
// their page in color, brightening to hover under the pointer, and then
// the text of after, as an album's name. An artist NetEase has no page for,
// credited by name alone, is plain text.
func (a *app) artistLinks(c *ui.Context, tr *netease.Track, color, hover ui.Color, after ...string) ui.Element {
	return ui.RichText(c).TextColor(color).Children(func() {
		for i, ar := range tr.Artists {
			if i > 0 {
				ui.Text(c, ", ")
			}
			if ar.ID == 0 {
				ui.Text(c, ar.Name)
				continue
			}
			link := ui.Link(c.Key(i), ar.Name, "").TextColor(color)
			if link.Hovered() {
				link.TextColor(hover)
			}
			// Once built: a song's row is in a list, which a new page resets.
			id := ar.ID
			link.OnClick(func() { a.openArtist(id) })
		}
		for _, s := range after {
			if s != "" {
				ui.Text(c, s)
			}
		}
	})
}

// albumAfter is what follows a track's artists where its album shows too.
func albumAfter(tr *netease.Track) string {
	if tr.Album.Name == "" {
		return ""
	}
	return " — " + tr.Album.Name
}

// openArtist opens an artist's page in the main window, from wherever
// their name shows: over Now Playing, which closes, or the notch.
func (a *app) openArtist(id int64) {
	a.open(route{page: pageArtist, artist: id})
	a.showMain()
	a.redraw()
}

// artistDetail and artistLatestSongs load an artist and their songs from
// NetEase; tests replace them.
var (
	artistDetail      = netease.ArtistDetail
	artistLatestSongs = netease.ArtistLatestSongs
)

// artistData is what the app has of an artist: who they are, with their
// most popular songs, and their newest songs once asked for.
type artistData struct {
	profile       *netease.ArtistProfile
	loading       bool
	err           string
	latest        []netease.Track
	latestLoading bool
	latestErr     string
}

// The orders an artist's page lists their songs in.
const (
	artistHot = iota
	artistLatest
)

// latestCount is how many of an artist's newest songs their page lists.
const latestCount = 100

func (a *app) artist(id int64) *artistData {
	d := a.artists[id]
	if d == nil {
		d = &artistData{}
		a.artists[id] = d
	}
	return d
}

// loadArtist loads an artist's page, and their newest songs too where
// those are what it lists.
func (a *app) loadArtist(id int64) {
	if a.artistSort == artistLatest {
		a.loadLatest(id)
	}
	d := a.artist(id)
	if d.profile != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		ar, err := artistDetail(id)
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err = "歌手加载失败"
				return
			}
			d.profile = ar
		})
	}()
}

func (a *app) loadLatest(id int64) {
	d := a.artist(id)
	if d.latest != nil || d.latestLoading {
		return
	}
	d.latestLoading, d.latestErr = true, ""
	go func() {
		songs, err := artistLatestSongs(id, latestCount)
		a.post(func() {
			d.latestLoading = false
			if err != nil {
				d.latestErr = "歌曲加载失败"
				return
			}
			d.latest = songs
		})
	}()
}

// artistPage shows who an artist is over their songs, the most popular or
// the newest.
func (a *app) artistPage(c *ui.Context) {
	id := a.route.artist
	d := a.artist(id)
	if d.profile == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadArtist(id) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.artistHeaderSkeleton(c, s) })
		return
	}
	header := func(c *ui.Context) { a.artistHeader(c, d) }
	songs := d.profile.Songs
	if a.artistSort == artistLatest {
		switch {
		case d.latest != nil:
			songs = d.latest
		case d.latestLoading:
			a.trackSkeleton(c, a.pageShimmer(c), true, header)
			return
		default:
			songs = nil
			header = func(c *ui.Context) {
				a.artistHeader(c, d)
				ui.Column(c).Padding(40, 0).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, d.latestErr).TextColor(a.pal.muted).FontSize(13)
					if ui.Button(c.Key("retry"), "重试").Clicked() {
						a.loadLatest(d.profile.ID)
					}
				})
			}
		}
	}
	a.trackRows(c, songs, true, header)
}

// artistPhotoSize is the width of an artist's round photo on their page.
const artistPhotoSize = 212

func (a *app) artistHeader(c *ui.Context, d *artistData) {
	p := a.pal
	ar := d.profile
	// What the buttons play: the songs listed.
	songs := ar.Songs
	if a.artistSort == artistLatest {
		songs = d.latest
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		photo := ar.Avatar
		if photo == "" {
			photo = ar.PicURL
		}
		ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, photo, 600, artistPhotoSize/2)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "歌手").FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, ar.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			var meta []string
			if aka := distinct(append([]string{ar.Trans}, ar.Alias...)); len(aka) > 0 {
				meta = append(meta, strings.Join(aka, " / "))
			}
			if ar.MusicSize > 0 {
				meta = append(meta, fmt.Sprintf("%d 首歌曲", ar.MusicSize))
			}
			if ar.AlbumSize > 0 {
				meta = append(meta, fmt.Sprintf("%d 张专辑", ar.AlbumSize))
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(p.muted)
			if d := strings.TrimSpace(ar.BriefDesc); d != "" {
				ui.Text(c, strings.ReplaceAll(d, "\n", " ")).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(songs, firstPlayable(songs))
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					if !a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(songs, firstPlayable(songs))
				}
			})
		})
	})
	ui.Row(c).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
		ui.Text(c, "歌曲").FontSize(19).FontWeight(700).LetterSpacing(-0.3)
		ui.Spacer(c)
		if segmented(c.Key("sort"), &a.artistSort, "热门", "最新") && a.artistSort == artistLatest {
			a.loadLatest(ar.ID)
		}
	})
}

func (a *app) artistHeaderSkeleton(c *ui.Context, s shimmer) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		s.bone(c, artistPhotoSize/2).Size(artistPhotoSize, artistPhotoSize)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			s.text(c, 12, 1.3).Width(24)
			s.text(c, 30, 1.2).WidthPercent(32)
			s.text(c, 13, 1.3).Width(160)
			ui.Column(c).Margin(2, 0, 0, 0).Children(func() {
				s.text(c, 12.5, 1.45).WidthPercent(78)
				s.text(c, 12.5, 1.45).WidthPercent(52)
			})
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				s.bone(c, 18).Size(92, 36)
				s.bone(c, 18).Size(120, 36)
			})
		})
	})
	a.section(c, "歌曲")
}

// distinct returns ss without empty strings, nor any after its first: an
// artist's translated name is often among their aliases too.
func distinct(ss []string) []string {
	var out []string
	for _, s := range ss {
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
