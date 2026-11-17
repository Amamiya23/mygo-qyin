package qqmusic

import (
	"encoding/json"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
)

func TestPlaylistSquareTabsAndTags(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Comm map[string]any `json:"comm"`
			Req  struct {
				Module string         `json:"module"`
				Method string         `json:"method"`
				Param  map[string]any `json:"param"`
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Comm["ct"] != float64(24) {
			t.Fatalf("square request used ct=%v, want 24", req.Comm["ct"])
		}
		switch req.Req.Method {
		case "GetSquareTab":
			rpcReply(w, map[string]any{
				"List": []any{
					map[string]any{"TagID": -1, "TagName": "推荐", "TagType": 1, "TagFrom": ""},
					map[string]any{"TagID": 3903, "TagName": "互动歌单", "TagType": 3, "TagFrom": ""},
					map[string]any{"TagID": 9527, "TagName": "AI歌单", "TagType": 5, "TagFrom": ""},
					map[string]any{"TagID": 3152, "TagName": "流行", "TagType": 3, "TagFrom": ""},
				},
			})
		case "GetAllTag":
			rpcReply(w, map[string]any{
				"v_group": []any{
					map[string]any{
						"group_id":   1,
						"group_name": "热门",
						"v_item": []any{
							map[string]any{"id": 3317, "name": "官方歌单", "source": 0},
							map[string]any{"id": 9527, "name": "AI歌单", "source": 1},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected method %s/%s", req.Req.Module, req.Req.Method)
		}
	})

	tabs, err := PlaylistSquareTabs()
	if err != nil {
		t.Fatal(err)
	}
	if len(tabs) != 4 || !tabs[0].IsRecommend() || !tabs[2].IsAI() {
		t.Fatalf("unexpected tabs: %+v", tabs)
	}

	groups, err := PlaylistAllTags()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Tags) != 2 || !groups[0].Tags[1].IsAI() {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}

func TestPlaylistSquareFeedRetriesRateLimit(t *testing.T) {
	var attempts atomic.Int32
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Req struct {
				Module string         `json:"module"`
				Method string         `json:"method"`
				Param  map[string]any `json:"param"`
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Req.Module != "music.playlist.PlaylistSquare" || req.Req.Method != "GetRecommendWhole" {
			t.Fatalf("unexpected RPC %s/%s", req.Req.Module, req.Req.Method)
		}
		if attempts.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 500003}})
			return
		}
		rpcReply(w, map[string]any{
			"HeadRsp": map[string]any{
				"List": []any{
					map[string]any{"Playlist": map[string]any{"basic": map[string]any{"tid": 1001, "title": "编辑精选", "play_cnt": 12000, "cover": map[string]any{"default_url": "https://example.com/h.jpg"}}}},
				},
			},
			"FeedRsp": map[string]any{
				"HasMore":   true,
				"FromLimit": 400,
				"List": []any{
					map[string]any{"Playlist": map[string]any{"basic": map[string]any{"tid": 2001, "title": "深夜轻音乐", "play_cnt": 54000, "cover": map[string]any{"medium_url": "https://example.com/f.jpg"}}}},
				},
			},
		})
	})

	page, err := PlaylistSquareFeed(0, 40, true)
	if err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("got %d attempts, want 2 after 500003 retry", attempts.Load())
	}
	if len(page.Head) != 1 || page.Head[0].ID != 1001 || len(page.Items) != 1 || page.Items[0].ID != 2001 || !page.HasMore || page.Limit != 400 {
		t.Fatalf("unexpected feed page: %+v", page)
	}
}

func TestCategoryEndpointsUseStringCallerAndAIBranch(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Req struct {
				Module string         `json:"module"`
				Method string         `json:"method"`
				Param  map[string]any `json:"param"`
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		switch req.Req.Method {
		case "get_category_basic":
			if _, ok := req.Req.Param["caller"].(string); !ok {
				t.Fatalf("get_category_basic caller must be string, got %T", req.Req.Param["caller"])
			}
			rpcReply(w, map[string]any{
				"name": "官方歌单",
				"v_child": []any{
					map[string]any{"id": 3317, "name": "全部"},
					map[string]any{"id": 3403, "name": "华语"},
				},
			})
		case "get_category_content":
			if req.Req.Module != "playlist.PlayListCategoryServer" {
				t.Fatalf("wrong module %s", req.Req.Module)
			}
			if _, ok := req.Req.Param["caller"].(string); !ok {
				t.Fatalf("get_category_content caller must be string, got %T", req.Req.Param["caller"])
			}
			rpcReply(w, map[string]any{
				"content": map[string]any{
					"total_cnt": 949,
					"v_item": []any{
						map[string]any{"basic": map[string]any{"tid": 9607499637, "title": "相信音乐官方精选", "play_cnt": 88000, "cover": map[string]any{"default_url": "https://example.com/c.jpg"}}},
					},
				},
			})
		case "get_ai_category_content":
			if req.Req.Module != "music.playlist.AiPlCategory" {
				t.Fatalf("wrong AI module %s", req.Req.Module)
			}
			rpcReply(w, map[string]any{
				"content": map[string]any{
					"total_cnt": 1,
					"v_item": []any{
						map[string]any{"basic": map[string]any{"tid": 8801, "title": "AI 晚安电台"}},
					},
				},
			})
		case "get_category_playtrack", "get_ai_category_playtrack":
			rpcReply(w, map[string]any{
				"songlist": []any{
					map[string]any{"id": 101, "mid": "songMid1", "title": "春日影", "interval": 240, "singer": []any{map[string]any{"id": 1, "mid": "a1", "name": "MyGO!!!!!"}}, "album": map[string]any{"id": 2, "mid": "al1", "name": "迷星叫"}},
				},
			})
		default:
			t.Fatalf("unexpected method %s/%s", req.Req.Module, req.Req.Method)
		}
	})

	basic, err := CategoryBasic(3317)
	if err != nil {
		t.Fatal(err)
	}
	if basic.Name != "官方歌单" || len(basic.Children) != 2 || basic.Children[1].ID != 3403 {
		t.Fatalf("unexpected basic: %+v", basic)
	}

	page, err := CategoryPlaylists(3317, 3317, 0, "", 60, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 9607499637 || page.LastID != "9607499637" || !page.HasMore {
		t.Fatalf("unexpected category page: %+v", page)
	}

	aiPage, err := CategoryPlaylists(9527, 9527, 0, "", 60, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(aiPage.Items) != 1 || aiPage.Items[0].ID != 8801 || aiPage.HasMore {
		t.Fatalf("unexpected AI category page: %+v", aiPage)
	}

	tracks, err := CategoryPlayTracks(3317, false)
	if err != nil || len(tracks) != 1 || tracks[0].Name != "春日影" {
		t.Fatalf("unexpected category tracks: %+v, err=%v", tracks, err)
	}
	aiTracks, err := CategoryPlayTracks(9527, true)
	if err != nil || len(aiTracks) != 1 || aiTracks[0].MID != "songMid1" {
		t.Fatalf("unexpected AI category tracks: %+v, err=%v", aiTracks, err)
	}
}

func TestLivePlaylistSquare(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})

	tabs, err := PlaylistSquareTabs()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tabs(%d): %+v", len(tabs), tabs)
	if len(tabs) < 8 {
		t.Fatalf("expected >=8 tabs with ct=24, got %d", len(tabs))
	}

	groups, err := PlaylistAllTags()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("groups(%d)", len(groups))
	if len(groups) == 0 {
		t.Fatal("empty tag groups")
	}

	feed, err := PlaylistSquareFeed(0, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("feed head=%d items=%d hasMore=%v limit=%d", len(feed.Head), len(feed.Items), feed.HasMore, feed.Limit)
	if len(feed.Items) == 0 {
		t.Fatal("empty feed items")
	}

	basic, err := CategoryBasic(3317)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("basic(3317): name=%s children=%d", basic.Name, len(basic.Children))
	if basic.Name == "" || len(basic.Children) == 0 {
		t.Fatalf("unexpected basic(3317): %+v", basic)
	}

	catPage, err := CategoryPlaylists(3317, 3317, 0, "", 20, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cat(3317): total=%d items=%d lastID=%s", catPage.Total, len(catPage.Items), catPage.LastID)
	if len(catPage.Items) == 0 {
		t.Fatal("empty cat(3317)")
	}

	aiPage0, err := CategoryPlaylists(9527, 9527, 0, "", 12, true)
	if err != nil {
		t.Fatal(err)
	}
	aiPage1, err := CategoryPlaylists(9527, 9527, 1, "", 12, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ai(9527): total=%d p0=%d p1=%d", aiPage0.Total, len(aiPage0.Items), len(aiPage1.Items))
	if len(aiPage0.Items) == 0 || len(aiPage1.Items) == 0 || aiPage0.Items[0].ID == aiPage1.Items[0].ID {
		t.Fatalf("unexpected AI pagination: p0=%d p1=%d", len(aiPage0.Items), len(aiPage1.Items))
	}

	tracks, err := CategoryPlayTracks(3317, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("playtrack(3317): %d tracks", len(tracks))
	if len(tracks) == 0 {
		t.Fatal("empty playtrack(3317)")
	}

	aiTracks, err := CategoryPlayTracks(9527, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ai playtrack(9527): %d tracks", len(aiTracks))
	if len(aiTracks) == 0 {
		t.Fatal("empty ai playtrack(9527)")
	}
}
