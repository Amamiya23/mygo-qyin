package player

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"lilt/internal/qqmusic"
)

const (
	coversSubdir    = "covers"
	lyricsSubdir    = "lyrics"
	playlistsSubdir = "playlists"
	purchasedSubdir = "purchased"
	coverExt        = ".img"
	lyricExt        = ".json"
	playlistExt     = ".json"
	purchasedExt    = ".json"
)

// CachedLyrics holds a track's raw lyric payloads kept on disk.
type CachedLyrics struct {
	LRC         string `json:"lrc,omitempty"`
	Translation string `json:"translation,omitempty"`
	Timed       string `json:"timed,omitempty"`
}

// CachedPlaylist holds a playlist detail snapshot and when it was fetched.
type CachedPlaylist struct {
	Playlist qqmusic.Playlist `json:"playlist"`
	CachedAt time.Time        `json:"cachedAt"`
}

// CachedUserPlaylists holds a user's made and saved playlist lists.
type CachedUserPlaylists struct {
	Made     []qqmusic.Playlist `json:"made,omitempty"`
	Saved    []qqmusic.Playlist `json:"saved,omitempty"`
	CachedAt time.Time          `json:"cachedAt"`
}

// CachedPurchasedSongs holds an account's cached purchased single tracks.
type CachedPurchasedSongs struct {
	Songs    []qqmusic.Track `json:"songs,omitempty"`
	More     bool            `json:"more,omitempty"`
	CachedAt time.Time       `json:"cachedAt"`
}

// CachedPurchasedAlbums holds an account's cached purchased albums.
type CachedPurchasedAlbums struct {
	Albums   []qqmusic.AlbumProfile `json:"albums,omitempty"`
	More     bool                   `json:"more,omitempty"`
	CachedAt time.Time              `json:"cachedAt"`
}

// Cache keeps the songs, covers, lyrics, playlists and purchased music on disk,
// so that they load and play at once, and without the network: songs as
// <id>-<level>.mp3 or .flac with <id>-<level>.json, covers under
// covers/<hash>.img, lyrics under lyrics/<id>.json, playlists under
// playlists/<id>.json, and purchased lists under purchased/*.json. Once they
// take more than the limit, those used longest ago go first.
type Cache struct {
	dir string

	mu      sync.Mutex
	limit   int64 // in bytes; 0 keeps nothing
	pruneMu sync.Mutex
}

// NewCache returns a cache in dir of limit bytes.
func NewCache(dir string, limit int64) *Cache {
	return &Cache{dir: dir, limit: limit}
}

// Directory returns the directory owned by this cache instance.
func (c *Cache) Directory() string { return c.dir }

// SetLimit changes how much the cache may take, and drops what is over.
func (c *Cache) SetLimit(limit int64) {
	c.mu.Lock()
	c.limit = limit
	c.mu.Unlock()
	c.prune("")
}

func cacheKey(id int64, level string) string { return fmt.Sprintf("%d-%s", id, level) }

func coverCacheKey(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) coverPath(url string) string {
	return filepath.Join(c.dir, coversSubdir, coverCacheKey(url)+coverExt)
}

func (c *Cache) lyricPath(id int64) string {
	return filepath.Join(c.dir, lyricsSubdir, strconv.FormatInt(id, 10)+lyricExt)
}

func (c *Cache) playlistPath(id int64) string {
	return filepath.Join(c.dir, playlistsSubdir, strconv.FormatInt(id, 10)+playlistExt)
}

func (c *Cache) userPlaylistsPath(uid int64) string {
	return filepath.Join(c.dir, playlistsSubdir, "user-"+strconv.FormatInt(uid, 10)+playlistExt)
}

func (c *Cache) purchasedSongsPath(uid int64) string {
	return filepath.Join(c.dir, purchasedSubdir, "songs-"+strconv.FormatInt(uid, 10)+purchasedExt)
}

func (c *Cache) purchasedAlbumsPath(uid int64) string {
	return filepath.Join(c.dir, purchasedSubdir, "albums-"+strconv.FormatInt(uid, 10)+purchasedExt)
}

func (c *Cache) currentLimit() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limit
}

// keepable reports whether a song of level and type ext may be kept: they
// name its file, so only those known, which keep it in the cache's folder.
func keepable(level, ext string) bool {
	return slices.Contains(qqmusic.Levels, level) && (ext == "mp3" || ext == "flac")
}

// lookup returns a song kept at level or a better one, open to read, with
// its stream.
func (c *Cache) lookup(id int64, level string) (*download, *qqmusic.Stream, bool) {
	if c == nil {
		return nil, nil, false
	}
	at := max(slices.Index(qqmusic.Levels, level), 0)
	for _, lv := range qqmusic.Levels[at:] {
		key := cacheKey(id, lv)
		meta, err := os.ReadFile(filepath.Join(c.dir, key+".json"))
		if err != nil {
			continue
		}
		var st qqmusic.Stream
		if json.Unmarshal(meta, &st) != nil || !keepable(lv, st.Type) {
			continue
		}
		path := filepath.Join(c.dir, key+"."+st.Type)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if info, err := f.Stat(); err != nil || info.Size() != st.Size {
			f.Close()
			continue
		}
		// Played now: the last to go.
		now := time.Now()
		os.Chtimes(path, now, now)
		return opened(f, st.Size), &st, true
	}
	return nil, nil, false
}

// LookupCover returns a cached cover image for url, refreshing its LRU timestamp.
func (c *Cache) LookupCover(url string) ([]byte, bool) {
	if c == nil || url == "" {
		return nil, false
	}
	path := c.coverPath(url)
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return data, true
}

// KeepCover stores a cover image for url on disk within the shared cache limit.
func (c *Cache) KeepCover(url string, data []byte) bool {
	if c == nil || url == "" || len(data) == 0 {
		return false
	}
	path, ok := c.writeAsset(c.coverPath(url), data)
	if ok {
		c.prune(path)
	}
	return ok
}

// DropCover removes any cached cover image for url.
func (c *Cache) DropCover(url string) {
	if c == nil || url == "" {
		return
	}
	os.Remove(c.coverPath(url))
}

// LookupLyrics returns cached lyrics for track id, refreshing its LRU timestamp.
func (c *Cache) LookupLyrics(id int64) (CachedLyrics, bool) {
	if c == nil || id <= 0 {
		return CachedLyrics{}, false
	}
	path := c.lyricPath(id)
	b, err := os.ReadFile(path)
	if err != nil {
		return CachedLyrics{}, false
	}
	var cached CachedLyrics
	if json.Unmarshal(b, &cached) != nil {
		return CachedLyrics{}, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return cached, true
}

// KeepLyrics stores lyrics for track id on disk within the shared cache limit.
func (c *Cache) KeepLyrics(id int64, lrc, translation, timed string) bool {
	if c == nil || id <= 0 {
		return false
	}
	b, err := json.Marshal(CachedLyrics{LRC: lrc, Translation: translation, Timed: timed})
	if err != nil {
		return false
	}
	path, ok := c.writeAsset(c.lyricPath(id), b)
	if ok {
		c.prune(path)
	}
	return ok
}

// MarshalPlaylist encodes pl with its fetch timestamp for disk storage.
func MarshalPlaylist(pl *qqmusic.Playlist) ([]byte, error) {
	return json.Marshal(CachedPlaylist{Playlist: *pl, CachedAt: time.Now()})
}

// LookupPlaylist returns a cached playlist detail and its fetch time,
// refreshing its LRU timestamp on disk.
func (c *Cache) LookupPlaylist(id int64) (*qqmusic.Playlist, time.Time, bool) {
	if c == nil || id <= 0 {
		return nil, time.Time{}, false
	}
	path := c.playlistPath(id)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false
	}
	var cached CachedPlaylist
	if json.Unmarshal(b, &cached) != nil || cached.Playlist.ID != id {
		return nil, time.Time{}, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	pl := cached.Playlist
	return &pl, cached.CachedAt, true
}

// KeepPlaylist stores a playlist detail snapshot on disk within the shared cache limit.
func (c *Cache) KeepPlaylist(pl *qqmusic.Playlist) bool {
	if c == nil || pl == nil || pl.ID <= 0 {
		return false
	}
	b, err := MarshalPlaylist(pl)
	if err != nil {
		return false
	}
	return c.KeepPlaylistData(pl.ID, b)
}

// KeepPlaylistData stores pre-marshaled playlist bytes on disk within the shared cache limit.
func (c *Cache) KeepPlaylistData(id int64, data []byte) bool {
	if c == nil || id <= 0 || len(data) == 0 {
		return false
	}
	path, ok := c.writeAsset(c.playlistPath(id), data)
	if ok {
		c.prune(path)
	}
	return ok
}

// DropPlaylist removes any cached detail for playlist id.
func (c *Cache) DropPlaylist(id int64) {
	if c == nil || id <= 0 {
		return
	}
	os.Remove(c.playlistPath(id))
}

// LookupUserPlaylists returns cached made and saved playlist lists for uid.
func (c *Cache) LookupUserPlaylists(uid int64) (made, saved []qqmusic.Playlist, ok bool) {
	if c == nil || uid <= 0 {
		return nil, nil, false
	}
	path := c.userPlaylistsPath(uid)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false
	}
	var cached CachedUserPlaylists
	if json.Unmarshal(b, &cached) != nil {
		return nil, nil, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return cached.Made, cached.Saved, true
}

// KeepUserPlaylists stores a user's made and saved playlist lists on disk.
func (c *Cache) KeepUserPlaylists(uid int64, made, saved []qqmusic.Playlist) bool {
	if c == nil || uid <= 0 {
		return false
	}
	b, err := json.Marshal(CachedUserPlaylists{Made: made, Saved: saved, CachedAt: time.Now()})
	if err != nil {
		return false
	}
	path, ok := c.writeAsset(c.userPlaylistsPath(uid), b)
	if ok {
		c.prune(path)
	}
	return ok
}

// DropUserPlaylists removes any cached playlist list for uid.
func (c *Cache) DropUserPlaylists(uid int64) {
	if c == nil || uid <= 0 {
		return
	}
	os.Remove(c.userPlaylistsPath(uid))
}

// MarshalPurchasedSongs encodes purchased single tracks with their fetch time.
func MarshalPurchasedSongs(songs []qqmusic.Track, more bool) ([]byte, error) {
	return json.Marshal(CachedPurchasedSongs{Songs: songs, More: more, CachedAt: time.Now()})
}

// LookupPurchasedSongs returns cached purchased single tracks for uid and refreshes
// the entry's disk LRU timestamp.
func (c *Cache) LookupPurchasedSongs(uid int64) (songs []qqmusic.Track, more bool, cachedAt time.Time, ok bool) {
	if c == nil || uid <= 0 {
		return nil, false, time.Time{}, false
	}
	path := c.purchasedSongsPath(uid)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false, time.Time{}, false
	}
	var cached CachedPurchasedSongs
	if json.Unmarshal(b, &cached) != nil {
		return nil, false, time.Time{}, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return cached.Songs, cached.More, cached.CachedAt, true
}

// KeepPurchasedSongs stores purchased single tracks for uid on disk.
func (c *Cache) KeepPurchasedSongs(uid int64, songs []qqmusic.Track, more bool) bool {
	if c == nil || uid <= 0 {
		return false
	}
	b, err := MarshalPurchasedSongs(songs, more)
	if err != nil {
		return false
	}
	return c.KeepPurchasedSongsData(uid, b)
}

// KeepPurchasedSongsData stores pre-marshaled purchased songs bytes on disk.
func (c *Cache) KeepPurchasedSongsData(uid int64, data []byte) bool {
	if c == nil || uid <= 0 || len(data) == 0 {
		return false
	}
	path, ok := c.writeAsset(c.purchasedSongsPath(uid), data)
	if ok {
		c.prune(path)
	}
	return ok
}

// MarshalPurchasedAlbums encodes purchased albums with their fetch time.
func MarshalPurchasedAlbums(albums []qqmusic.AlbumProfile, more bool) ([]byte, error) {
	return json.Marshal(CachedPurchasedAlbums{Albums: albums, More: more, CachedAt: time.Now()})
}

// LookupPurchasedAlbums returns cached purchased albums for uid and refreshes
// the entry's disk LRU timestamp.
func (c *Cache) LookupPurchasedAlbums(uid int64) (albums []qqmusic.AlbumProfile, more bool, cachedAt time.Time, ok bool) {
	if c == nil || uid <= 0 {
		return nil, false, time.Time{}, false
	}
	path := c.purchasedAlbumsPath(uid)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false, time.Time{}, false
	}
	var cached CachedPurchasedAlbums
	if json.Unmarshal(b, &cached) != nil {
		return nil, false, time.Time{}, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return cached.Albums, cached.More, cached.CachedAt, true
}

// KeepPurchasedAlbums stores purchased albums for uid on disk.
func (c *Cache) KeepPurchasedAlbums(uid int64, albums []qqmusic.AlbumProfile, more bool) bool {
	if c == nil || uid <= 0 {
		return false
	}
	b, err := MarshalPurchasedAlbums(albums, more)
	if err != nil {
		return false
	}
	return c.KeepPurchasedAlbumsData(uid, b)
}

// KeepPurchasedAlbumsData stores pre-marshaled purchased albums bytes on disk.
func (c *Cache) KeepPurchasedAlbumsData(uid int64, data []byte) bool {
	if c == nil || uid <= 0 || len(data) == 0 {
		return false
	}
	path, ok := c.writeAsset(c.purchasedAlbumsPath(uid), data)
	if ok {
		c.prune(path)
	}
	return ok
}

// DropPurchased removes cached purchased songs and albums for uid.
func (c *Cache) DropPurchased(uid int64) {
	if c == nil || uid <= 0 {
		return
	}
	os.Remove(c.purchasedSongsPath(uid))
	os.Remove(c.purchasedAlbumsPath(uid))
}

func (c *Cache) writeAsset(path string, data []byte) (string, bool) {
	limit := c.currentLimit()
	if limit <= 0 || int64(len(data)) > limit/4 {
		return "", false
	}
	dir := filepath.Dir(path)
	if os.MkdirAll(dir, 0o700) != nil {
		return "", false
	}
	tmp, err := os.CreateTemp(dir, "*.part")
	if err != nil {
		return "", false
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return "", false
	}
	if err := tmp.Close(); err != nil {
		return "", false
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", false
	}
	cleanupTmp = false
	return path, true
}

// download fetches the song of the stream st into a file, to keep once it
// has all come in, or a temporary file if the cache would not keep it.
// Memory is only the fallback when no file can be made or written.
func (c *Cache) download(ctx context.Context, id int64, st *qqmusic.Stream) *download {
	part := c.part(st)
	if part == nil {
		part, _ = temporaryDownloadFile()
		return startDownload(ctx, st.URL, st.Size, part, nil, nil)
	}
	return startDownload(ctx, st.URL, st.Size, part, func(path string, n int64) (string, bool) {
		return c.keep(id, st, path, n)
	}, c.prune)
}

// part makes a file for a song of the stream st to come into, or returns
// nil if the cache would not keep it. Each has a name of its own: the same
// song played again while it still comes in has another.
func (c *Cache) part(st *qqmusic.Stream) *os.File {
	if c == nil {
		return nil
	}
	limit := c.currentLimit()
	// A song of more than a quarter of it would push out too many.
	if limit <= 0 || st.Size > limit/4 {
		return nil
	}
	if os.MkdirAll(c.dir, 0o700) != nil {
		return nil
	}
	f, err := os.CreateTemp(c.dir, "*.part")
	if err != nil {
		return nil
	}
	return f
}

// keep keeps the song of the stream st, n bytes, from the file part, which
// is closed. It returns where the song is: kept, or still at part.
// Pruning waits until the download has reopened it and released its lock.
func (c *Cache) keep(id int64, st *qqmusic.Stream, part string, n int64) (string, bool) {
	limit := c.currentLimit()
	if n > limit/4 {
		return part, false
	}
	kept := *st
	if kept.Type == "" {
		kept.Type = "mp3"
	}
	kept.Size = n
	if !keepable(kept.Level, kept.Type) {
		return part, false
	}
	key := cacheKey(id, kept.Level)
	meta, _ := json.Marshal(kept)
	path := filepath.Join(c.dir, key+"."+kept.Type)
	if os.Rename(part, path) != nil {
		return part, false
	}
	os.WriteFile(filepath.Join(c.dir, key+".json"), meta, 0o600)
	return path, true
}

// Size returns how many bytes the cached songs, covers and lyrics take.
func (c *Cache) Size() int64 {
	var n int64
	for _, f := range c.items() {
		n += f.size
	}
	return n
}

// Clear drops every song, cover and lyric kept.
func (c *Cache) Clear() {
	if c == nil {
		return
	}
	c.pruneMu.Lock()
	defer c.pruneMu.Unlock()
	for _, f := range c.items() {
		c.remove(f)
	}
}

type cachedItem struct {
	path   string
	size   int64
	used   time.Time
	isSong bool
}

func (c *Cache) songs() []cachedItem {
	var out []cachedItem
	entries, _ := os.ReadDir(c.dir)
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext != ".mp3" && ext != ".flac" {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, cachedItem{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime(), true})
	}
	return out
}

func (c *Cache) items() []cachedItem {
	out := c.songs()
	out = appendDirItems(out, filepath.Join(c.dir, coversSubdir), coverExt)
	out = appendDirItems(out, filepath.Join(c.dir, lyricsSubdir), lyricExt)
	out = appendDirItems(out, filepath.Join(c.dir, playlistsSubdir), playlistExt)
	out = appendDirItems(out, filepath.Join(c.dir, purchasedSubdir), purchasedExt)
	return out
}

func appendDirItems(out []cachedItem, dir, wantExt string) []cachedItem {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) != wantExt {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, cachedItem{filepath.Join(dir, e.Name()), info.Size(), info.ModTime(), false})
	}
	return out
}

func (c *Cache) remove(item cachedItem) {
	os.Remove(item.path)
	if item.isSong {
		os.Remove(strings.TrimSuffix(item.path, filepath.Ext(item.path)) + ".json")
	}
}

// prune drops the entries used longest ago until the rest fit the limit,
// but keep, the entry just kept.
func (c *Cache) prune(keep string) {
	if c == nil {
		return
	}
	c.pruneMu.Lock()
	defer c.pruneMu.Unlock()
	limit := c.currentLimit()
	c.dropParts()
	items := c.items()
	slices.SortFunc(items, func(a, b cachedItem) int { return a.used.Compare(b.used) })
	var total int64
	for _, f := range items {
		total += f.size
	}
	for _, f := range items {
		if total <= limit {
			return
		}
		if f.path != keep {
			c.remove(f)
			total -= f.size
		}
	}
}

// dropParts removes temporary partial files left by an app that quit mid-write.
func (c *Cache) dropParts() {
	for _, dir := range []string{c.dir, filepath.Join(c.dir, coversSubdir), filepath.Join(c.dir, lyricsSubdir), filepath.Join(c.dir, playlistsSubdir), filepath.Join(c.dir, purchasedSubdir)} {
		parts, _ := filepath.Glob(filepath.Join(dir, "*.part"))
		for _, path := range parts {
			if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > time.Hour {
				os.Remove(path)
			}
		}
	}
}
