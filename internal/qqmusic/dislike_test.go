package qqmusic

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestReadSongBlacklistPagesThroughSongIDs(t *testing.T) {
	const first = 9007199254740993
	calls := make(chan int, 50)
	fakeLibraryModuleRPC(t, func(module, method string, params map[string]json.RawMessage) any {
		page := int(number(params, "Page"))
		calls <- page
		if module != dislikeModule || method != "GetDislikeList" || number(params, "Cmd") != 3 {
			t.Errorf("unexpected blacklist operation: %s/%s %v", module, method, params)
		}
		var songs = []any{}
		switch page {
		case 1:
			if string(params["SongLastid"]) != "0" {
				t.Errorf("first page cursor: %s", params["SongLastid"])
			}
			songs = append(songs, map[string]any{"ID": "9007199254740993"})
		case 2:
			if text(params, "SongLastid") != "9007199254740993" {
				t.Errorf("second page cursor: %s", params["SongLastid"])
			}
			songs = append(songs, map[string]any{"ID": 42})
		case 3:
			if text(params, "SongLastid") != "42" {
				t.Errorf("last page cursor: %s", params["SongLastid"])
			}
		default:
			t.Errorf("unexpected page %d", page)
		}
		return map[string]any{"Retcode": 0, "Page": strconv.Itoa(page), "Songs": songs, "Singers": []any{map[string]any{"ID": "43"}}}
	})
	if _, err := ReadSongBlacklist(10002); !errors.Is(err, ErrLoginRequired) || len(calls) != 0 {
		t.Fatalf("read for another account: %v", err)
	}
	got, err := ReadSongBlacklist(10001)
	if err != nil || !got.Complete || !maps.Equal(got.IDs, map[int64]struct{}{first: {}, 42: {}}) {
		t.Fatalf("list=%+v, err=%v", got, err)
	}
	if len(calls) != 3 || <-calls != 1 || <-calls != 2 || <-calls != 3 {
		t.Fatal("blacklist pages were not read in order")
	}
}

func TestReadSongBlacklistKeepsConfirmedIDsButDoesNotAssumeCoverage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		second map[string]any
	}{
		{"server failure", map[string]any{"Retcode": 5, "Page": 2, "Songs": []any{}}},
		{"missing song list", map[string]any{"Retcode": 0, "Page": 2}},
		{"unverified end page", map[string]any{"Retcode": 0, "Songs": []any{}}},
		{"server repeated first page", map[string]any{"Retcode": 0, "Page": 1, "Songs": []any{map[string]string{"ID": "42"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			fakeLibraryRPC(t, func(method string, _ map[string]json.RawMessage) any {
				if method != "GetDislikeList" {
					t.Errorf("read-only sync called %q", method)
				}
				switch calls.Add(1) {
				case 1:
					return map[string]any{"Retcode": 0, "Page": 1, "Songs": []any{map[string]string{"ID": "42"}}}
				case 2:
					return tc.second
				default:
					t.Error("continued reading after an ambiguous page")
					return nil
				}
			})
			got, _ := ReadSongBlacklist(10001)
			if got.Complete || !maps.Equal(got.IDs, map[int64]struct{}{42: {}}) || calls.Load() != 2 {
				t.Fatalf("list=%+v, calls=%d", got, calls.Load())
			}
		})
	}
}

func TestDislikeSongConfirmsOnTheWritingAccount(t *testing.T) {
	const uid, song = 10001, 9007199254740993
	calls := make(chan string, 8)
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Comm struct{ QQ, Authst string }
			Call struct {
				Module, Method string
				Param          map[string]any
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		calls <- req.Call.Method
		key, _ := r.Cookie("qqmusic_key")
		if req.Call.Module != "music.feedback.FeedbackBlack" || req.Comm.QQ != "10001" || req.Comm.Authst != "first" || key == nil || key.Value != "first" {
			t.Errorf("request %s did not use the writing account", req.Call.Method)
		}
		switch req.Call.Method {
		case "AddDislike":
			want := map[string]any{"Songs": []any{map[string]any{"ID": "9007199254740993"}}}
			if !reflect.DeepEqual(req.Call.Param, want) {
				t.Errorf("write parameters: %v", req.Call.Param)
			}
			setSession(Session{MusicID: "10002", MusicKey: "second", LoginType: 2})
			rpcReply(w, map[string]any{"Retcode": 0})
		case "GetDislikeList":
			want := map[string]any{"Cmd": float64(3), "Page": float64(1), "SongLastid": float64(0), "SingersLastid": float64(0)}
			if !reflect.DeepEqual(req.Call.Param, want) {
				t.Errorf("read parameters: %v", req.Call.Param)
			}
			rpcReply(w, map[string]any{"Retcode": 0, "Page": "", "Songs": []map[string]string{{"ID": "9007199254740993"}}})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	if err := DislikeSong(uid, song); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("anonymous write: %v", err)
	}
	setSession(Session{MusicID: "10001", MusicKey: "first", LoginType: 2})
	if err := DislikeSong(uid+1, song); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("write for a different account: %v", err)
	}
	if err := DislikeSong(uid, song); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || <-calls != "AddDislike" || <-calls != "GetDislikeList" {
		t.Fatal("expected one write followed by one read")
	}
}

func TestDislikeSongDistinguishesRejectionFromUnconfirmed(t *testing.T) {
	for _, tc := range []struct {
		name        string
		write, read any
		unconfirmed bool
		reads       int32
	}{
		{"rejected", map[string]any{"Retcode": 42}, nil, false, 0},
		{"missing write status", map[string]any{}, nil, true, 0},
		{"not on first page", map[string]any{"Retcode": 0}, map[string]any{"Retcode": 0, "Songs": []any{}}, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes, reads atomic.Int32
			fakeLibraryRPC(t, func(method string, _ map[string]json.RawMessage) any {
				if method == "AddDislike" {
					writes.Add(1)
					return tc.write
				}
				if method == "GetDislikeList" {
					reads.Add(1)
					return tc.read
				}
				t.Errorf("unexpected method %q", method)
				return nil
			})
			err := DislikeSong(10001, 123)
			if err == nil || errors.Is(err, ErrDislikeUnconfirmed) != tc.unconfirmed || writes.Load() != 1 || reads.Load() != tc.reads {
				t.Fatalf("err=%v writes=%d reads=%d", err, writes.Load(), reads.Load())
			}
		})
	}
}

func TestRecommendationProvenanceStaysOutOfSharedCatalog(t *testing.T) {
	fakeLibraryRPC(t, func(_ string, _ map[string]json.RawMessage) any {
		entries := []any{map[string]any{"id": 92201, "mid": "recommendationMid"}}
		return map[string]any{"songlist": entries, "tracks": entries}
	})
	for _, tc := range []struct {
		daily bool
		read  func() ([]Track, error)
	}{{true, DailySongs}, {false, func() ([]Track, error) { return PersonalFM(FMDefault) }}} {
		tracks, err := tc.read()
		if err != nil || len(tracks) != 1 || tracks[0].RecommendationAccount != 10001 || tracks[0].FromDailyRecommendations != tc.daily {
			t.Fatalf("tracks=%+v err=%v", tracks, err)
		}
		// Restoring a saved queue remembers its tracks without turning the
		// same song into a recommendation in unrelated catalog views.
		RememberTracks(tracks)
		if track, ok := knownTrack(92201); !ok || track.RecommendationAccount != 0 || track.FromDailyRecommendations {
			t.Fatalf("catalog track: %+v", track)
		}
	}
}
