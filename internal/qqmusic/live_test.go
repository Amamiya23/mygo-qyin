package qqmusic

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"

	"lilt/internal/lyrics"
)

// Song comments must load from the current web service and advance past the first page.
func TestLiveSongComments(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	for _, id := range []string{"105816212", "102150080"} {
		t.Run(id, func(t *testing.T) {
			first, err := ThreadComments(id, 0, 30)
			if err != nil {
				t.Fatal(err)
			}
			if first.Total <= 0 || len(first.Hot) == 0 || len(first.Latest) == 0 || !first.More {
				t.Fatalf("incomplete first page: %+v", first)
			}
			next, err := ThreadComments(id, len(first.Latest), 30)
			if err != nil {
				t.Fatal(err)
			}
			if len(next.Latest) == 0 {
				t.Fatal("empty second page")
			}
			seen := map[string]bool{}
			for _, cm := range first.Latest {
				seen[cm.RemoteID] = true
			}
			for _, cm := range next.Latest {
				if cm.RemoteID == "" || seen[cm.RemoteID] {
					t.Fatal("next page repeated a comment or lost its identity")
				}
			}
			t.Logf("total=%d hot=%d latest=%d next=%d", first.Total, len(first.Hot), len(first.Latest), len(next.Latest))
		})
	}
}

// Singer discographies must remain readable and paginate while an account is present.
func TestLiveArtistAlbums(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	artists, err := SearchPage("徐佳莹", SearchArtists, 5, 0)
	if err != nil || len(artists.Artists) == 0 {
		t.Fatalf("artist search: %v", err)
	}
	for _, account := range []string{"anonymous", "QQ", "WeChat"} {
		t.Run(account, func(t *testing.T) {
			setSession(Session{})
			if account != "anonymous" {
				loginType := 2
				if account == "WeChat" {
					loginType = 1
				}
				// Exercise account-independent reads without using a real credential.
				setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: loginType})
			}
			var firstID int64
			for _, offset := range []int{0, 5} {
				albums, more, err := ArtistAlbums(artists.Artists[0].ID, 5, offset)
				if err != nil {
					t.Fatal(err)
				}
				if len(albums) == 0 || albums[0].ID <= 0 || albums[0].MID == "" || albums[0].Name == "" || !more {
					t.Fatalf("missing discography metadata or pagination: %+v, more=%v", albums, more)
				}
				if offset > 0 && albums[0].ID == firstID {
					t.Fatal("discography repeated its first page")
				}
				firstID = albums[0].ID
				t.Logf("%s offset=%d: %d albums, more=%v", artists.Artists[0].Name, offset, len(albums), more)
			}
		})
	}
}

// Anonymous search must return usable songs, artists and albums when QQ allows the public endpoint.
func TestLiveSearch(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	for _, kind := range []SearchType{SearchSongs, SearchArtists, SearchAlbums} {
		result, err := SearchPage("周杰伦", kind, 20, 0)
		if err != nil {
			t.Fatalf("anonymous kind=%d: %v", kind, err)
		}
		if result.Len() == 0 || result.Total == 0 {
			t.Fatalf("anonymous kind=%d: empty public search", kind)
		}
		var id int64
		var mid string
		switch kind {
		case SearchSongs:
			id, mid = result.Songs[0].ID, result.Songs[0].MID
		case SearchArtists:
			id, mid = result.Artists[0].ID, result.Artists[0].MID
		case SearchAlbums:
			id, mid = result.Albums[0].ID, result.Albums[0].MID
		}
		if id <= 0 || mid == "" {
			t.Fatalf("anonymous kind=%d: missing catalog identifiers", kind)
		}
		t.Logf("anonymous kind=%d: %d of %d", kind, result.Len(), result.Total)
	}
}

// QQ's fixed-size CDN thumbnails must decode at the sizes used by rows, records and Now Playing.
func TestLiveArtworkSizes(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	for _, cover := range []struct{ name, url string }{
		{"album", albumCover("0037d8b14AvEnt")},
		{"artist", artistCover("004P4J0D0bAzf3")},
	} {
		t.Run(cover.name, func(t *testing.T) {
			for _, size := range []int{96, 240, 600} {
				b, err := Fetch(Sized(cover.url, size))
				if err != nil {
					t.Fatalf("size %d: %v", size, err)
				}
				config, _, err := image.DecodeConfig(bytes.NewReader(b))
				if err != nil || config.Width <= 0 || config.Height <= 0 {
					t.Fatalf("size %d: invalid image %+v: %v", size, config, err)
				}
				t.Logf("requested %d: decoded %dx%d", size, config.Width, config.Height)
			}
		})
	}
}

// The installed client reads public QQ catalogs and a real stream without a local API service.
func TestLivePublicQQ(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	if sessionSnapshot().MusicKey != "" {
		t.Fatal("live public test requires anonymous state")
	}
	featured, err := FeaturedPlaylists(3)
	if err != nil {
		t.Fatal("featured:", err)
	}
	if len(featured) == 0 {
		t.Fatal("no featured playlists")
	}
	t.Logf("featured: %d", len(featured))
	p, err := PlaylistDetail(featured[0].ID)
	if err != nil {
		t.Fatal("playlist:", err)
	}
	if len(p.Tracks) == 0 {
		t.Fatal("playlist has no tracks")
	}
	t.Logf("playlist: %d tracks", len(p.Tracks))
	charts, err := Charts()
	if err != nil {
		t.Fatal("charts:", err)
	}
	if len(charts) == 0 {
		t.Fatal("no charts")
	}
	chart, err := PlaylistDetail(charts[0].ID)
	if err != nil {
		t.Fatal("chart detail:", err)
	}
	if len(chart.Tracks) == 0 {
		t.Fatal("chart has no tracks")
	}
	t.Logf("charts: %d, first has %d tracks", len(charts), len(chart.Tracks))
	search, err := SearchPage("周杰伦", SearchSongs, 10, 0)
	if err != nil {
		t.Fatal("search:", err)
	}
	if len(search.Songs) == 0 {
		t.Fatal("search has no songs")
	}
	t.Logf("search: %d of %d", len(search.Songs), search.Total)
	albumSearch, err := SearchPage("周杰伦", SearchAlbums, 5, 0)
	if err != nil {
		t.Fatal("album search:", err)
	}
	if len(albumSearch.Albums) == 0 || albumSearch.Albums[0].ID == 0 || albumSearch.Albums[0].MID == "" {
		t.Fatal("album search metadata missing")
	}
	artistSearch, err := SearchPage("周杰伦", SearchArtists, 5, 0)
	if err != nil {
		t.Fatal("artist search:", err)
	}
	if len(artistSearch.Artists) == 0 || artistSearch.Artists[0].ID == 0 || artistSearch.Artists[0].MID == "" {
		t.Fatal("artist search metadata missing")
	}
	song := search.Songs[0]
	details, err := SongDetails([]int64{song.ID})
	if err != nil {
		t.Fatal("song detail:", err)
	}
	if len(details) != 1 || details[0].MID != song.MID {
		t.Fatal("song detail mismatch")
	}
	lrc, _, _, err := Lyrics(song.ID)
	if err != nil {
		t.Fatal("lyrics:", err)
	}
	t.Logf("lyrics: %d bytes", len(lrc))
	album, err := AlbumDetail(song.Album.ID)
	if err != nil {
		t.Fatal("album:", err)
	}
	if len(album.Songs) == 0 {
		t.Fatal("album has no songs")
	}
	if album.Name == "" || album.MID == "" || album.PicURL == "" {
		t.Fatalf("album metadata missing: %+v", album.Album)
	}
	t.Logf("album: %d tracks", len(album.Songs))
	artist, err := ArtistDetail(song.Artists[0].ID)
	if err != nil {
		t.Fatal("artist:", err)
	}
	if len(artist.Songs) == 0 {
		t.Fatal("artist has no tracks")
	}
	if artist.ID == 0 || artist.MID == "" || artist.Name == "" {
		t.Fatal("artist metadata missing")
	}
	t.Logf("artist: %d tracks", len(artist.Songs))
	albums, _, err := ArtistAlbums(song.Artists[0].ID, 10, 0)
	if err != nil {
		t.Fatal("artist albums:", err)
	}
	t.Logf("artist albums: %d", len(albums))
	if len(albums) == 0 || albums[0].ID == 0 || albums[0].MID == "" || albums[0].Name == "" {
		t.Fatalf("artist album metadata missing: %+v", albums)
	}
	var streamed bool
	for _, candidate := range append(search.Songs, p.Tracks[:min(len(p.Tracks), 5)]...) {
		stream, err := SongURL(candidate.ID, "standard")
		if err != nil {
			t.Logf("stream unavailable: %v", err)
			continue
		}
		request, err := http.NewRequest(http.MethodGet, stream.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Range", "bytes=0-1023")
		request.Header.Set("User-Agent", UserAgent)
		response, err := httpClient.Do(request)
		if err != nil {
			t.Log("CDN:", err)
			continue
		}
		b, readErr := io.ReadAll(io.LimitReader(response.Body, 1024))
		response.Body.Close()
		if (response.StatusCode != 200 && response.StatusCode != 206) || readErr != nil {
			t.Logf("CDN status=%d error=%v", response.StatusCode, readErr)
			continue
		}
		if len(b) == 0 {
			t.Fatal("CDN returned empty stream")
		}
		if stream.Size <= 0 {
			t.Fatal("stream lacks size required for seeking")
		}
		t.Logf("stream: HTTP %d, %s, read %d bytes", response.StatusCode, stream.Type, len(b))
		streamed = true
		break
	}
	if !streamed {
		t.Fatal("no public stream verified")
	}
}

// QQ's public login endpoint must return an actual PNG and a waiting QR status.
func TestLiveQQQRCode(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1 to contact QQ Music")
	}
	key, err := QRKey()
	if err != nil {
		t.Fatal(err)
	}
	png, err := QRImage(key)
	if err != nil || len(png) < 8 {
		t.Fatalf("QR image: %d bytes, %v", len(png), err)
	}
	code, _, _, _, err := QRCheck(key)
	if err != nil {
		t.Fatal(err)
	}
	if code != QRWaiting {
		t.Fatalf("new QR status %d", code)
	}
	t.Logf("QQ returned a %d-byte QR image and waiting status", len(png))
}

// The WeChat web flow must bootstrap a device, return an image and accept an anonymous status poll.
func TestLiveWechatQRCode(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1 to contact QQ Music")
	}
	key, err := WechatQRKey()
	if err != nil {
		t.Fatal(err)
	}
	defer CancelQR(key)
	image, err := QRImage(key)
	if err != nil || len(image) < 8 {
		t.Fatalf("QR image: %d bytes, %v", len(image), err)
	}
	code, _, _, _, err := QRCheck(key)
	if err != nil {
		t.Fatal(err)
	}
	if code != QRWaiting {
		t.Fatalf("new QR status %d", code)
	}
	t.Logf("WeChat returned a %d-byte QR image and waiting status", len(image))
}

// Every advertised song chart must return usable tracks rather than a successful empty custom page.
func TestLiveSongCharts(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	// Exercise public chart reads while an account is present without using real credentials.
	setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2})
	charts, err := Charts()
	if err != nil || len(charts) == 0 {
		t.Fatalf("chart catalog: count=%d, error=%v", len(charts), err)
	}
	for _, chart := range charts {
		t.Run(strconv.FormatInt(chart.ID-chartIDBase, 10), func(t *testing.T) {
			detail, err := PlaylistDetail(chart.ID)
			if err != nil {
				t.Fatalf("%s: %v", chart.Name, err)
			}
			if detail.Name == "" || len(detail.Tracks) == 0 {
				t.Fatalf("missing chart contents: %+v", detail)
			}
			for _, track := range detail.Tracks {
				if track.ID <= 0 || track.MID == "" || track.Name == "" {
					t.Fatalf("unusable chart track: %+v", track)
				}
			}
			t.Logf("%s: %d songs", detail.Name, len(detail.Tracks))
		})
	}
}

// QQ's public song detail must return usable measurements without account credentials.
func TestLiveSongLoudness(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	isolateLoudnessCache(t)
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	volume, err := SongLoudness(Track{MID: "0039MnYb0qxYhV"})
	if err != nil || volume.Peak <= 0 || volume.Gain == 0 {
		t.Fatalf("missing loudness measurement: %+v, error=%v", volume, err)
	}
	t.Logf("gain=%g dB peak=%g lra=%g LU", volume.Gain, volume.Peak, volume.LRA)
}

// Anonymous lyric requests must decode real word times all the way into the player's model.
func TestLiveLyricsWordTimes(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	for _, song := range []Track{
		{ID: 97773, MID: "0039MnYb0qxYhV"},
		{ID: 1180947, MID: "002pXw073bnbSj"},
	} {
		t.Run(song.MID, func(t *testing.T) {
			RememberTracks([]Track{song})
			lrc, trans, qrc, err := Lyrics(song.ID)
			if err != nil {
				t.Fatal(err)
			}
			lines := lyrics.Build(lrc, trans, qrc)
			wordCount := 0
			for _, line := range lines {
				if line.Timed {
					wordCount += len(line.Words)
				}
			}
			if qrc == "" || wordCount == 0 {
				t.Fatalf("real word timing unavailable: lrc bytes=%d qrc bytes=%d lines=%d", len(lrc), len(qrc), len(lines))
			}
			if song.ID == 1180947 && trans == "" {
				t.Fatal("the Japanese sample lost its translation")
			}
			t.Logf("decoded %d timed words in %d lines; translation bytes=%d", wordCount, len(lines), len(trans))
		})
	}
}

// Line-only lyrics and instrumental placeholders must not be mistaken for real word timing.
func TestLiveLyricsLineFallback(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	for _, mid := range []string{"003XGaNw3g8rjL", "001nnncU0VsH5O"} {
		t.Run(mid, func(t *testing.T) {
			lrc, trans, qrc, err := playLyrics(mid)
			if err != nil || lrc == "" || qrc != "" {
				t.Fatalf("unexpected line-only response: lrc bytes=%d qrc bytes=%d error=%v", len(lrc), len(qrc), err)
			}
			lines := lyrics.Build(lrc, trans, qrc)
			for _, line := range lines {
				if line.Timed {
					t.Fatalf("invented word timing: %+v", line)
				}
			}
			if mid == "001nnncU0VsH5O" && len(lines) != 0 {
				t.Fatalf("instrumental placeholder shown as lyrics: %d lines", len(lines))
			}
			t.Logf("decoded line-only payload, visible lines=%d", len(lines))
		})
	}
}
