package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"lilt/internal/control"
	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

// The commands of other programs (lilt ctl, lilt mcp): see package control.
// Each runs on the socket's goroutine, reading and changing the app on the
// main thread through onMain, and waiting on QQ Music off it.

func (a *app) serveControl() (stop func()) {
	stop, err := control.Serve(a.control)
	if err != nil {
		log.Printf("control: %v", err)
		return func() {}
	}
	return stop
}

// These ask QQ Music; tests replace them.
var (
	tracksByID     = qqmusic.TracksByID
	playlistDetail = qqmusic.PlaylistDetail
)

var errSignedOut = errors.New("not signed in: sign in to QQ Music in the app first")

// onMain runs fn on the main thread and waits for its result.
func onMain[T any](a *app, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	a.win.Update(func() {
		v, err := fn()
		done <- result{v, err}
		a.redraw()
	})
	select {
	case r := <-done:
		return r.v, r.err
	case <-time.After(10 * time.Second):
		var zero T
		return zero, errors.New("the app did not answer in time")
	}
}

func (a *app) run(fn func()) error {
	_, err := onMain(a, func() (struct{}, error) { fn(); return struct{}{}, nil })
	return err
}

func (a *app) signedInUser() (int64, error) {
	return onMain(a, func() (int64, error) {
		if a.profile == nil {
			return 0, errSignedOut
		}
		return a.profile.UserID, nil
	})
}

// What commands answer: compact, for agents to read.

type trackOut struct {
	ID       int64   `json:"id"`
	MID      string  `json:"mid,omitempty"`
	Name     string  `json:"name"`
	Artists  string  `json:"artists"`
	Album    string  `json:"album,omitempty"`
	AlbumID  int64   `json:"album_id,omitempty"`
	Duration float64 `json:"duration_sec"`
	Playable bool    `json:"playable"`
}

func trackInfo(t *qqmusic.Track) trackOut {
	return trackOut{ID: t.ID, MID: t.MID, Name: t.Name, Artists: t.ArtistNames(), Album: t.Album.Name, AlbumID: t.Album.ID,
		Duration: seconds(t.Length()), Playable: t.Playable()}
}

func tracksInfo(ts []qqmusic.Track) []trackOut {
	out := make([]trackOut, len(ts))
	for i := range ts {
		out[i] = trackInfo(&ts[i])
	}
	return out
}

func seconds(d time.Duration) float64 { return math.Round(d.Seconds()*10) / 10 }

type playlistOut struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	TrackCount int    `json:"track_count"`
	// Kind is liked (我喜欢), created or saved; only created ones are editable.
	Kind    string `json:"kind"`
	Creator string `json:"creator,omitempty"`
}

func page[T any](items []T, offset, limit int64, def int64) ([]T, error) {
	if offset < 0 || limit < 0 {
		return nil, errors.New("offset and limit must not be negative")
	}
	if limit == 0 {
		limit = def
	}
	start := min(int(offset), len(items))
	return items[start:min(start+int(limit), len(items))], nil
}

var repeatNames = map[player.RepeatMode]string{player.RepeatOff: "off", player.RepeatAll: "all", player.RepeatOne: "one"}

type controlArgs struct {
	Query      string  `json:"query"`
	Type       string  `json:"type"`
	Offset     int64   `json:"offset"`
	Limit      int64   `json:"limit"`
	ID         int64   `json:"id"`
	SongIDs    []int64 `json:"song_ids"`
	PlaylistID int64   `json:"playlist_id"`
	AlbumID    int64   `json:"album_id"`
	Index      *int64  `json:"index"`
	Seconds    float64 `json:"seconds"`
	Level      int64   `json:"level"`
	On         bool    `json:"on"`
	Mode       string  `json:"mode"`
	SongID     int64   `json:"song_id"`
	Liked      *bool   `json:"liked"`
	Name       string  `json:"name"`
}

func (a *app) control(cmd string, raw json.RawMessage) (any, error) {
	var args controlArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	switch cmd {
	case "status":
		return onMain(a, a.status)
	case "account":
		return onMain(a, func() (any, error) {
			if a.profile == nil {
				return map[string]any{"signed_in": false}, nil
			}
			return map[string]any{"signed_in": true, "user_id": a.profile.UserID, "nickname": a.profile.Nickname, "avatar_url": a.profile.AvatarURL}, nil
		})
	case "search":
		return a.ctlSearch(args)
	case "playlists":
		return a.ctlPlaylists()
	case "playlist":
		if args.ID == 0 {
			return nil, errors.New("id is required")
		}
		pl, err := playlistDetail(args.ID)
		if err != nil {
			return nil, err
		}
		tracks, err := page(pl.Tracks, args.Offset, args.Limit, 50)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": pl.ID, "name": pl.Name, "description": pl.Description, "creator": pl.Creator.Nickname,
			"track_count": max(pl.TrackCount, len(pl.Tracks)), "offset": args.Offset, "tracks": tracksInfo(tracks)}, nil
	case "album":
		al, err := qqmusic.AlbumDetail(args.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": al.ID, "name": al.Name, "artists": al.ArtistNames(), "description": al.Description,
			"publish_time": al.PublishTime, "tracks": tracksInfo(al.Songs)}, nil
	case "artist":
		ar, err := qqmusic.ArtistDetail(args.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": ar.ID, "name": ar.Name, "brief": ar.BriefDesc, "song_count": ar.MusicSize,
			"album_count": ar.AlbumSize, "hot_songs": tracksInfo(ar.Songs)}, nil
	case "play":
		return a.ctlPlay(args)
	case "pause", "resume":
		want := cmd == "resume"
		return onMain(a, func() (any, error) {
			if a.pl.Current() == nil && len(a.pl.Queue()) == 0 {
				return nil, errors.New("nothing to play: the queue is empty")
			}
			if a.pl.Playing() != want {
				a.pl.TogglePlay()
			}
			return a.status()
		})
	case "next", "prev":
		return onMain(a, func() (any, error) {
			if len(a.pl.Queue()) == 0 {
				return nil, errors.New("the queue is empty")
			}
			if cmd == "next" {
				a.pl.Next()
			} else {
				a.pl.Prev()
			}
			return a.status()
		})
	case "seek":
		return onMain(a, func() (any, error) {
			if a.pl.Current() == nil {
				return nil, errors.New("nothing is playing")
			}
			if args.Seconds < 0 {
				return nil, errors.New("seconds must not be negative")
			}
			a.pl.Seek(time.Duration(args.Seconds * float64(time.Second)))
			return a.status()
		})
	case "volume":
		if args.Level < 0 || args.Level > 100 {
			return nil, errors.New("level must be 0–100")
		}
		return onMain(a, func() (any, error) {
			a.volume = float64(args.Level) / 100
			a.pl.SetVolume(a.volume)
			return a.status()
		})
	case "shuffle":
		return onMain(a, func() (any, error) {
			if a.pl.Shuffle() != args.On {
				a.pl.ToggleShuffle()
			}
			return a.status()
		})
	case "repeat":
		var mode player.RepeatMode = -1
		for m, name := range repeatNames {
			if name == args.Mode {
				mode = m
			}
		}
		if mode < 0 {
			return nil, errors.New("mode must be off, all or one")
		}
		return onMain(a, func() (any, error) {
			for a.pl.Repeat() != mode {
				a.pl.CycleRepeat()
			}
			return a.status()
		})
	case "queue":
		return onMain(a, func() (any, error) {
			queue := a.pl.Queue()
			order, at := a.pl.PlayOrder()
			current := -1
			if at >= 0 && at < len(order) {
				current = order[at]
			}
			tracks, err := page(queue, args.Offset, args.Limit, 50)
			if err != nil {
				return nil, err
			}
			return map[string]any{"length": len(queue), "current_index": current, "offset": args.Offset, "tracks": tracksInfo(tracks)}, nil
		})
	case "jump":
		if args.Index == nil {
			return nil, errors.New("index is required")
		}
		return onMain(a, func() (any, error) {
			i := int(*args.Index)
			if i < 0 || i >= len(a.pl.Queue()) {
				return nil, fmt.Errorf("index must be 0–%d", len(a.pl.Queue())-1)
			}
			a.pl.Jump(i)
			return a.status()
		})
	case "like":
		return a.ctlLike(args)
	case "playlist_create":
		return a.ctlCreate(args)
	case "playlist_rename":
		return a.ctlRename(args)
	case "playlist_delete":
		return a.ctlDelete(args)
	case "playlist_add", "playlist_remove":
		return a.ctlEditSongs(args, cmd == "playlist_add")
	}
	return nil, fmt.Errorf("unknown command %s", cmd)
}

func (a *app) status() (any, error) {
	out := map[string]any{
		"signed_in": a.profile != nil,
		"playing":   a.pl.Playing(),
		"loading":   a.pl.Loading(),
		"volume":    int(math.Round(a.volume * 100)),
		"shuffle":   a.pl.Shuffle(),
		"repeat":    repeatNames[a.pl.Repeat()],
		"queue_len": len(a.pl.Queue()),
	}
	if a.profile != nil {
		out["user"] = map[string]any{"user_id": a.profile.UserID, "nickname": a.profile.Nickname}
	}
	if tr := a.pl.Current(); tr != nil {
		t := trackInfo(tr)
		out["track"] = t
		out["liked"] = a.isLiked(tr)
		out["position_sec"] = seconds(a.pl.Position())
		if d := a.pl.Duration(); d > 0 {
			out["duration_sec"] = seconds(d)
		}
	}
	if order, at := a.pl.PlayOrder(); at >= 0 && at < len(order) {
		out["queue_index"] = order[at]
	}
	if err := a.pl.Err(); err != "" {
		out["error"] = err
	}
	return out, nil
}

func (a *app) ctlSearch(args controlArgs) (any, error) {
	if args.Query == "" {
		return nil, errors.New("query is required")
	}
	kinds := map[string]qqmusic.SearchType{"": qqmusic.SearchSongs, "song": qqmusic.SearchSongs, "album": qqmusic.SearchAlbums, "artist": qqmusic.SearchArtists}
	kind, ok := kinds[args.Type]
	if !ok {
		return nil, errors.New("type must be song, album or artist")
	}
	limit := int(args.Limit)
	if limit <= 0 {
		limit = 20
	}
	r, err := qqmusic.SearchPage(args.Query, kind, min(limit, 50), int(max(args.Offset, 0)))
	if err != nil {
		return nil, err
	}
	out := map[string]any{"total": r.Total}
	switch kind {
	case qqmusic.SearchSongs:
		out["songs"] = tracksInfo(r.Songs)
	case qqmusic.SearchAlbums:
		albums := make([]map[string]any, len(r.Albums))
		for i, al := range r.Albums {
			albums[i] = map[string]any{"id": al.ID, "name": al.Name, "artists": al.ArtistNames(), "publish_time": al.PublishTime, "size": al.Size}
		}
		out["albums"] = albums
	case qqmusic.SearchArtists:
		artists := make([]map[string]any, len(r.Artists))
		for i, ar := range r.Artists {
			artists[i] = map[string]any{"id": ar.ID, "name": ar.Name, "song_count": ar.MusicSize, "album_count": ar.AlbumSize}
		}
		out["artists"] = artists
	}
	return out, nil
}

// ctlPlaylists loads the listener's playlists afresh, and shows them in the
// sidebar too.
func (a *app) ctlPlaylists() (any, error) {
	uid, err := a.signedInUser()
	if err != nil {
		return nil, err
	}
	made, saved, err := qqmusic.UserPlaylists(uid)
	if err != nil {
		return nil, err
	}
	_ = a.run(func() {
		if a.profile != nil && a.profile.UserID == uid {
			a.mine, a.saved = made, saved
		}
	})
	out := make([]playlistOut, 0, len(made)+len(saved))
	for _, pl := range made {
		kind := "created"
		if pl.SpecialType == 5 {
			kind = "liked"
		}
		out = append(out, playlistOut{ID: pl.ID, Name: pl.Name, TrackCount: pl.TrackCount, Kind: kind})
	}
	for _, pl := range saved {
		out = append(out, playlistOut{ID: pl.ID, Name: pl.Name, TrackCount: pl.TrackCount, Kind: "saved", Creator: pl.Creator.Nickname})
	}
	return out, nil
}

func (a *app) ctlPlay(args controlArgs) (any, error) {
	sources := 0
	for _, set := range []bool{len(args.SongIDs) > 0, args.PlaylistID != 0, args.AlbumID != 0} {
		if set {
			sources++
		}
	}
	if sources != 1 {
		return nil, errors.New("give exactly one of song_ids, playlist_id or album_id")
	}
	var tracks []qqmusic.Track
	var skipped []int64
	start := -1
	switch {
	case len(args.SongIDs) > 0:
		if args.Index != nil && (*args.Index < 0 || int(*args.Index) >= len(args.SongIDs)) {
			return nil, fmt.Errorf("index must be 0–%d", len(args.SongIDs)-1)
		}
		got, err := tracksByID(args.SongIDs)
		if err != nil {
			return nil, err
		}
		byID := make(map[int64]qqmusic.Track, len(got))
		for _, t := range got {
			byID[t.ID] = t
		}
		// A queue goes on without songs QQ Music has no details of.
		for i, id := range args.SongIDs {
			t, ok := byID[id]
			if !ok {
				skipped = append(skipped, id)
				if args.Index != nil && int(*args.Index) == i {
					return nil, fmt.Errorf("song %d at index %d was not found", id, i)
				}
				continue
			}
			if args.Index != nil && int(*args.Index) == i {
				start = len(tracks)
			}
			tracks = append(tracks, t)
		}
	case args.PlaylistID != 0:
		pl, err := playlistDetail(args.PlaylistID)
		if err != nil {
			return nil, err
		}
		tracks = pl.Tracks
	default:
		al, err := qqmusic.AlbumDetail(args.AlbumID)
		if err != nil {
			return nil, err
		}
		tracks = al.Songs
	}
	if len(tracks) == 0 {
		return nil, errors.New("no songs found to play")
	}
	switch {
	case start >= 0:
	case args.Index == nil:
		start = firstPlayable(tracks)
	case *args.Index < 0 || int(*args.Index) >= len(tracks):
		return nil, fmt.Errorf("index must be 0–%d", len(tracks)-1)
	default:
		start = int(*args.Index)
	}
	return onMain(a, func() (any, error) {
		a.pl.PlayQueue(tracks, start)
		out := map[string]any{"queue_len": len(tracks), "track": trackInfo(&tracks[start])}
		if len(skipped) > 0 {
			out["skipped_not_found"] = skipped
		}
		return out, nil
	})
}

func (a *app) ctlLike(args controlArgs) (any, error) {
	if args.SongID <= 0 {
		return nil, errors.New("song_id is required")
	}
	if _, err := a.signedInUser(); err != nil {
		return nil, err
	}
	like := args.Liked == nil || *args.Liked
	if err := likeSong(args.SongID, like); err != nil {
		return nil, err
	}
	_ = a.run(func() {
		if a.liked == nil {
			a.liked = map[int64]bool{}
		}
		a.setLiked(args.SongID, like)
		a.likedSongsChanged()
	})
	return map[string]any{"song_id": args.SongID, "liked": like}, nil
}

func (a *app) ctlCreate(args controlArgs) (any, error) {
	if _, err := a.signedInUser(); err != nil {
		return nil, err
	}
	args.SongIDs = unique(args.SongIDs)
	// Before making the playlist, which bad IDs would leave empty.
	if len(args.SongIDs) > 0 {
		if err := songsExist(args.SongIDs); err != nil {
			return nil, err
		}
	}
	pl, err := createPlaylist(args.Name)
	if err != nil {
		return nil, err
	}
	id := pl.ID
	_ = a.run(func() {
		a.addMine(pl)
		a.toast = "已创建歌单“" + args.Name + "”"
	})
	out := map[string]any{"id": id, "name": args.Name}
	if len(args.SongIDs) > 0 {
		if err := addToPlaylist(id, args.SongIDs...); err != nil {
			return nil, fmt.Errorf("playlist %d was created, but adding songs failed: %w", id, err)
		}
		_ = a.run(func() { a.playlistChanged(id) })
		out["added"] = len(args.SongIDs)
	}
	return out, nil
}

func (a *app) ctlRename(args controlArgs) (any, error) {
	if _, err := a.signedInUser(); err != nil {
		return nil, err
	}
	if err := renamePlaylist(args.ID, args.Name); err != nil {
		return nil, err
	}
	_ = a.run(func() {
		a.setPlaylistName(args.ID, args.Name)
		a.toast = "歌单已重命名为“" + args.Name + "”"
	})
	return map[string]any{"id": args.ID, "name": args.Name}, nil
}

func (a *app) ctlDelete(args controlArgs) (any, error) {
	type target struct {
		name  string
		saved bool
	}
	t, err := onMain(a, func() (target, error) {
		if a.profile == nil {
			return target{}, errSignedOut
		}
		if pl := a.savedPlaylist(args.ID); pl != nil {
			return target{pl.Name, true}, nil
		}
		for _, pl := range a.mine {
			if pl.ID == args.ID {
				if pl.SpecialType == 5 {
					return target{}, errors.New("the liked songs playlist cannot be deleted")
				}
				return target{pl.Name, false}, nil
			}
		}
		return target{}, fmt.Errorf("playlist %d is not in the library: run playlists to refresh it", args.ID)
	})
	if err != nil {
		return nil, err
	}
	remove := deletePlaylist
	if t.saved {
		remove = unfavoritePlaylist
	}
	if err := remove(args.ID); err != nil {
		return nil, err
	}
	_ = a.run(func() {
		a.takePlaylist(args.ID, t.saved)
		delete(a.playlists, args.ID)
		a.toast = "已删除歌单"
		if t.saved {
			a.toast = "已取消收藏"
		}
	})
	action := "deleted"
	if t.saved {
		action = "unsaved"
	}
	return map[string]any{"id": args.ID, "name": t.name, "action": action}, nil
}

func (a *app) ctlEditSongs(args controlArgs, add bool) (any, error) {
	if _, err := a.signedInUser(); err != nil {
		return nil, err
	}
	if len(args.SongIDs) == 0 {
		return nil, errors.New("song_ids is required")
	}
	ids := unique(args.SongIDs)
	if add {
		// QQ Music keeps IDs of no song, which the playlist then counts.
		if err := songsExist(ids); err != nil {
			return nil, err
		}
	}
	// QQ Music refuses, without saying why, songs already in the playlist,
	// or not in it to remove.
	pl, err := playlistDetail(args.ID)
	if err != nil {
		return nil, err
	}
	in := make(map[int64]bool, len(pl.Tracks))
	for _, t := range pl.Tracks {
		in[t.ID] = true
	}
	var change, skipped []int64
	for _, id := range ids {
		if in[id] != add {
			change = append(change, id)
		} else {
			skipped = append(skipped, id)
		}
	}
	key, skipKey := "removed", "not_in_playlist"
	if add {
		key, skipKey = "added", "already_in_playlist"
	}
	out := map[string]any{"id": args.ID, key: len(change)}
	if len(skipped) > 0 {
		out[skipKey] = skipped
	}
	if len(change) == 0 {
		return out, nil
	}
	edit := removeFromPlaylist
	if add {
		edit = addToPlaylist
	}
	if err := edit(args.ID, change...); err != nil {
		return nil, err
	}
	_ = a.run(func() {
		a.playlistChanged(args.ID)
		a.toast = "已从歌单中移除"
		if add {
			a.toast = "已添加到歌单"
		}
	})
	return out, nil
}

func songsExist(ids []int64) error {
	got, err := tracksByID(ids)
	if err != nil {
		return err
	}
	found := make(map[int64]bool, len(got))
	for _, t := range got {
		found[t.ID] = true
	}
	var missing []int64
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("no song with ID %v", missing)
	}
	return nil
}

func unique(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	var out []int64
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
