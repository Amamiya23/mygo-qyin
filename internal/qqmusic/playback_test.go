package qqmusic

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// A restored queue must retain MIDs and the chosen file size so seeking need not wait for EOF.
func TestPlaybackRestoredSizeAndQualityFallback(t *testing.T) {
	track := convertTrack(obj(json.RawMessage(`{"id":993,"mid":"restoredSong","title":"Song","file":{"media_mid":"restoredMedia","size_128mp3":123456,"size_320mp3":345678,"size_flac":987654}}`)))
	encoded, err := json.Marshal([]Track{track})
	if err != nil {
		t.Fatal(err)
	}
	var restored []Track
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || restored[0].MID != "restoredSong" || restored[0].MediaMID != "restoredMedia" || restored[0].Size128 != 123456 || restored[0].Size320 != 345678 || restored[0].SizeFLAC != 987654 {
		t.Fatalf("queue metadata lost: %+v", restored)
	}
	RememberTracks(restored)
	for _, served := range []struct {
		prefix, level string
		size          int64
	}{{"F000", "lossless", 987654}, {"M800", "exhigh", 345678}, {"M500", "standard", 123456}} {
		t.Run(served.level, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Req struct {
						Module, Method string
						Param          struct {
							Filename []string `json:"filename"`
						} `json:"param"`
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.Req.Module != "music.vkey.GetVkey" || request.Req.Method != "UrlGetVkey" {
					t.Errorf("wrong authenticated vkey request: %+v", request)
				}
				filename := request.Req.Param.Filename[0]
				if !strings.Contains(filename, "restoredSongrestoredMedia") {
					t.Errorf("restored MIDs lost: %s", filename)
				}
				purl := ""
				if strings.HasPrefix(filename, served.prefix) {
					purl = "file"
				}
				rpcReply(w, map[string]any{"sip": []string{"https://dl.stream.qqmusic.qq.com/"}, "midurlinfo": []any{map[string]any{"songmid": "restoredSong", "purl": purl}}})
			})
			setSession(Session{MusicID: "123", MusicKey: "credential", LoginType: 2})
			stream, err := SongURL(993, "lossless")
			if err != nil {
				t.Fatal(err)
			}
			if stream.Level != served.level || stream.Size != served.size {
				t.Fatalf("quality and size disagree: %+v", stream)
			}
		})
	}
}

// QQ requires both the song MID and media MID; a one-segment filename can get a CDN 403.
func TestPlaybackFilenameAndUnavailable(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "unavailable"}[available], func(t *testing.T) {
			RememberTracks([]Track{{ID: 991, MID: "playSong", MediaMID: "playMedia"}})
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Req struct {
						Module, Method string
						Param          struct {
							Filename []string `json:"filename"`
							SongMID  []string `json:"songmid"`
						} `json:"param"`
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if request.Req.Module != "vkey.GetVkeyServer" || request.Req.Method != "CgiGetVkey" || len(request.Req.Param.Filename) != 1 || request.Req.Param.Filename[0] != "M500playSongplayMedia.mp3" {
					t.Errorf("wrong playback request: %+v", request)
				}
				purl := ""
				if available {
					purl = "M500file.mp3?vkey=test"
				}
				rpcReply(w, map[string]any{"sip": []string{"https://dl.stream.qqmusic.qq.com/"}, "midurlinfo": []any{map[string]any{"songmid": "playSong", "purl": purl}}})
			})
			stream, err := SongURL(991, "hires")
			if !available {
				if !errors.Is(err, ErrUnavailable) {
					t.Fatalf("got %v, want unavailable", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if stream.URL != "https://dl.stream.qqmusic.qq.com/M500file.mp3?vkey=test" || stream.Level != "standard" || stream.Type != "mp3" {
				t.Fatalf("wrong stream %+v", stream)
			}
		})
	}
}

// Missing media MID repeats the song MID instead of requesting a one-segment filename.
func TestPlaybackMissingMediaMID(t *testing.T) {
	RememberTracks([]Track{{ID: 992, MID: "onlyMid"}})
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Req struct {
				Param struct {
					Filename []string `json:"filename"`
				} `json:"param"`
			} `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if len(request.Req.Param.Filename) != 1 || request.Req.Param.Filename[0] != "M500onlyMidonlyMid.mp3" {
			t.Errorf("wrong filename %+v", request)
		}
		rpcReply(w, map[string]any{"sip": []string{"https://dl.stream.qqmusic.qq.com/"}, "midurlinfo": []any{map[string]any{"songmid": "onlyMid", "purl": "file.mp3"}}})
	})
	if _, err := SongURL(992, "standard"); err != nil {
		t.Fatal(err)
	}
}
