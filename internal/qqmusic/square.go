package qqmusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// SquareTab is a top navigation tab in QQ Music's playlist square.
type SquareTab struct {
	ID   int64  `json:"TagID"`
	Name string `json:"TagName"`
	Type int    `json:"TagType"`
	From string `json:"TagFrom"`
}

func (t SquareTab) IsRecommend() bool {
	return t.ID == -1 || t.Type == 1
}

func (t SquareTab) IsAI() bool {
	return t.Type == 5 || t.ID == 9527
}

// CategoryTag is a playlist category tag in the full category directory or child list.
type CategoryTag struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Cover  string `json:"cover,omitempty"`
	Type   int    `json:"type,omitempty"`
	Status int    `json:"status,omitempty"`
	Source int    `json:"source,omitempty"`
}

func (t CategoryTag) IsAI() bool {
	return t.Source == 1 || t.ID == 9527
}

// CategoryGroup groups category tags by theme, mood, genre, etc.
type CategoryGroup struct {
	ID   int64         `json:"group_id"`
	Name string        `json:"group_name"`
	Tags []CategoryTag `json:"v_item"`
}

// CategoryInfo describes a parent category and its optional sub-categories.
type CategoryInfo struct {
	ID       int64
	Name     string
	Children []CategoryTag
}

// SquareFeedPage is a page of playlists from the playlist square recommend feed.
type SquareFeedPage struct {
	Head    []Playlist
	Items   []Playlist
	HasMore bool
	Limit   int
}

// CategoryPlaylistsPage is a cursor page of playlists within a category.
type CategoryPlaylistsPage struct {
	Items   []Playlist
	Total   int
	LastID  string
	HasMore bool
}

func squareComm() (map[string]any, string) {
	s := sessionSnapshot()
	uin := "0"
	if s.MusicID != "" && s.MusicKey != "" {
		uin = s.MusicID
	}
	// GetSquareTab requires ct != 20 (ct=24 returns all 8 tabs including Recommend/Interactive/AI).
	comm := map[string]any{
		"format":      "json",
		"ct":          24,
		"cv":          2276,
		"platform":    "wk_v17",
		"inCharset":   "utf-8",
		"outCharset":  "utf-8",
		"notice":      0,
		"needNewCode": 1,
		"uin":         uin,
	}
	return comm, uin
}

func rpcSquare(ctx context.Context, module, method string, param any, out any) error {
	comm, _ := squareComm()
	var err error
	for attempt := range 3 {
		err = rpcWithCommContext(ctx, module, method, param, out, comm, nil)
		var proto *ProtocolError
		if err == nil || !errors.As(err, &proto) || (proto.Code != 500003 && proto.GlobalCode != 500003) {
			return err
		}
		if attempt == 2 {
			break
		}
		delay := time.Duration(attempt+1) * 300 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func parseSquarePlaylist(entry object) Playlist {
	basic := obj(entry["basic"])
	if len(basic) == 0 {
		basic = obj(obj(entry["Playlist"])["basic"])
	}
	if len(basic) == 0 {
		basic = entry
	}
	p := Playlist{
		ID:          number(basic, "tid", "dissid", "id"),
		Name:        text(basic, "title", "dissname", "name"),
		Description: text(basic, "desc", "description"),
		TrackCount:  int(number(basic, "song_cnt", "song_num", "songnum")),
		PlayCount:   float64(number(basic, "play_cnt", "listen_num", "listennum")),
	}
	if cover := obj(basic["cover"]); len(cover) > 0 {
		p.CoverURL = text(cover, "default_url", "big_url", "medium_url", "small_url")
	}
	if p.CoverURL == "" {
		p.CoverURL = text(basic, "cover_url", "imgurl", "picurl", "logo")
	}
	if creator := obj(basic["creator"]); len(creator) > 0 {
		p.Creator.Nickname = text(creator, "nick", "nickname", "name")
	}
	return p
}

// PlaylistSquareTabs returns the top tabs of the playlist square.
func PlaylistSquareTabs() ([]SquareTab, error) {
	var data struct {
		List []SquareTab `json:"List"`
	}
	if err := rpcSquare(context.Background(), "music.playlist.PlaylistSquare", "GetSquareTab", map[string]any{}, &data); err != nil {
		return nil, err
	}
	out := make([]SquareTab, 0, len(data.List))
	for _, tab := range data.List {
		if tab.Name != "" {
			out = append(out, tab)
		}
	}
	return out, nil
}

// PlaylistAllTags returns all playlist category groups and tags.
func PlaylistAllTags() ([]CategoryGroup, error) {
	var data struct {
		Groups []CategoryGroup `json:"v_group"`
	}
	if err := rpcSquare(context.Background(), "music.playlist.PlaylistSquare", "GetAllTag", map[string]any{"qq": ""}, &data); err != nil {
		return nil, err
	}
	out := make([]CategoryGroup, 0, len(data.Groups))
	for _, g := range data.Groups {
		if g.Name == "" {
			continue
		}
		tags := make([]CategoryTag, 0, len(g.Tags))
		for _, t := range g.Tags {
			if t.ID > 0 && t.Name != "" {
				tags = append(tags, t)
			}
		}
		if len(tags) > 0 {
			g.Tags = tags
			out = append(out, g)
		}
	}
	return out, nil
}

// PlaylistSquareFeed loads a page from the playlist square's recommend feed.
func PlaylistSquareFeed(from, size int, includeHead bool) (SquareFeedPage, error) {
	if from < 0 {
		from = 0
	}
	size = min(max(size, 1), 200)
	var data object
	param := map[string]any{
		"IsReqHead": includeHead,
		"IsReqFeed": true,
		"FeedReq": map[string]any{
			"From": from,
			"Size": size,
		},
	}
	if err := rpcSquare(context.Background(), "music.playlist.PlaylistSquare", "GetRecommendWhole", param, &data); err != nil {
		return SquareFeedPage{}, err
	}
	var page SquareFeedPage
	if head := obj(data["HeadRsp"]); len(head) > 0 {
		for _, entry := range objects(head, "List") {
			if pl := parseSquarePlaylist(entry); pl.ID > 0 {
				page.Head = append(page.Head, pl)
			}
		}
	}
	feed := obj(data["FeedRsp"])
	for _, entry := range objects(feed, "List") {
		if pl := parseSquarePlaylist(entry); pl.ID > 0 {
			page.Items = append(page.Items, pl)
		}
	}
	var hasMore bool
	_ = json.Unmarshal(feed["HasMore"], &hasMore)
	if !hasMore {
		hasMore = number(feed, "HasMore") == 1
	}
	page.Limit = int(number(feed, "FromLimit"))
	page.HasMore = hasMore && len(page.Items) > 0
	return page, nil
}

// CategoryBasic returns the category name and sub-categories for categoryID.
func CategoryBasic(categoryID int64) (CategoryInfo, error) {
	if categoryID <= 0 {
		return CategoryInfo{}, fmt.Errorf("qqmusic: invalid category id %d", categoryID)
	}
	_, uin := squareComm()
	var data object
	param := map[string]any{
		"caller":      uin,
		"category_id": categoryID,
	}
	if err := rpcSquare(context.Background(), "playlist.PlayListCategoryServer", "get_category_basic", param, &data); err != nil {
		return CategoryInfo{}, err
	}
	cat := obj(data["category"])
	if len(cat) == 0 {
		cat = data
	}
	info := CategoryInfo{
		ID:   categoryID,
		Name: text(cat, "name", "title"),
	}
	for _, item := range objects(cat, "v_child") {
		child := CategoryTag{
			ID:     number(item, "id", "category_id", "cid"),
			Name:   text(item, "name", "title"),
			Source: int(number(item, "source")),
		}
		if child.ID > 0 && child.Name != "" {
			info.Children = append(info.Children, child)
		}
	}
	return info, nil
}

// CategoryPlaylists loads a page of playlists under categoryID.
// Normal categories paginate by lastID cursor; AI categories (ai=true) paginate by zero-based page index.
func CategoryPlaylists(categoryID, titleID int64, page int, lastID string, size int, ai bool) (CategoryPlaylistsPage, error) {
	if categoryID <= 0 {
		return CategoryPlaylistsPage{}, fmt.Errorf("qqmusic: invalid category id %d", categoryID)
	}
	if titleID <= 0 {
		titleID = categoryID
	}
	if page < 0 {
		page = 0
	}
	size = min(max(size, 1), 100)
	_, uin := squareComm()
	module, method := "playlist.PlayListCategoryServer", "get_category_content"
	param := map[string]any{
		"caller":      uin,
		"category_id": categoryID,
		"titleid":     titleID,
		"size":        size,
		"order":       5,
		"is_parent":   0,
	}
	if ai {
		module, method = "music.playlist.AiPlCategory", "get_ai_category_content"
		param["page"] = page
		param["use_page"] = 1
	} else {
		param["last_id"] = lastID
	}
	var data object
	if err := rpcSquare(context.Background(), module, method, param, &data); err != nil {
		return CategoryPlaylistsPage{}, err
	}
	content := obj(data["content"])
	if len(content) == 0 {
		content = data
	}
	items := objects(content, "v_item")
	out := make([]Playlist, 0, len(items))
	var nextLastID string
	for _, entry := range items {
		pl := parseSquarePlaylist(entry)
		if pl.ID > 0 {
			out = append(out, pl)
			nextLastID = strconv.FormatInt(pl.ID, 10)
		}
	}
	total := int(number(content, "total_cnt"))
	var hasMore bool
	if ai {
		hasMore = len(out) > 0 && (total == 0 && len(out) >= size || total > 0 && (page+1)*size < total)
	} else {
		hasMore = len(out) > 0 && nextLastID != "" && nextLastID != lastID
		if total > 0 && len(out) >= total && lastID == "" {
			hasMore = false
		}
	}
	return CategoryPlaylistsPage{
		Items:   out,
		Total:   total,
		LastID:  nextLastID,
		HasMore: hasMore,
	}, nil
}

// CategoryPlayTracks loads playable tracks for "play all" in a category.
func CategoryPlayTracks(categoryID int64, ai bool) ([]Track, error) {
	if categoryID <= 0 {
		return nil, fmt.Errorf("qqmusic: invalid category id %d", categoryID)
	}
	module, method := "music.playlist.PlayListCategory", "get_category_playtrack"
	if ai {
		module, method = "music.playlist.AiPlCategory", "get_ai_category_playtrack"
	}
	var data object
	if err := rpcSquare(context.Background(), module, method, map[string]any{"category_id": categoryID}, &data); err != nil {
		return nil, err
	}
	entries := objects(data, "songlist", "track_list", "tracks", "v_song", "v_track", "list", "song_list", "vec_song", "v_item")
	for i, entry := range entries {
		if nested := obj(entry["song"]); len(nested) > 0 {
			entries[i] = nested
		} else if nested := obj(entry["track"]); len(nested) > 0 {
			entries[i] = nested
		}
	}
	return convertTracks(entries), nil
}
