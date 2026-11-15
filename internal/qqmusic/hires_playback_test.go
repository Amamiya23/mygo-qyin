package qqmusic

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHiResPlaybackAndFallback(t *testing.T) {
	for _, mode := range []string{"hires", "permission", "empty", "403", "404", "HTML", "low resolution", "320k", "128k", "connection", "old queue"} {
		t.Run(mode, func(t *testing.T) {
			previous, prior := httpClient, sessionSnapshot()
			t.Cleanup(func() { httpClient = previous; setSession(prior) })
			setSession(Session{MusicID: "123", MusicKey: "fixture-key", LoginType: 1})
			track := Track{ID: 8891, MID: "hiresSong", MediaMID: "hiresMedia", SizeHiRes: 1000, SizeFLAC: 800, Size320: 600, Size128: 400}
			if mode == "old queue" {
				track.SizeHiRes = 0
			}
			RememberTracks([]Track{track})
			var requested []string
			details := 0
			httpClient = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodGet {
					if !strings.Contains(r.URL.Path, "RS01") || r.Header.Get("Range") != "bytes=0-65535" {
						t.Fatal("unexpected stream probe")
					}
					if mode == "connection" {
						return nil, errors.New("connection failed")
					}
					status, body := 206, hiResTestHeader()
					switch mode {
					case "403":
						status = 403
					case "404":
						status = 404
					case "HTML":
						body = []byte("<html>unavailable</html>")
					case "low resolution":
						body[20] &^= 1
						body[21] = body[21]&0x0f | 0xf0 // 16-bit instead of 24-bit.
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Range": {"bytes 0-41/123456"}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
				}
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          struct {
							Filename  []string
							Uin       string
							Loginflag int
							Platform  string
						}
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if request.Comm["tmeLoginType"] != float64(1) || request.Comm["authst"] != "fixture-key" || request.Comm["qq"] != "123" {
					t.Fatal("WeChat authentication was not preserved")
				}
				response := func(data any, code int) (*http.Response, error) {
					b, _ := json.Marshal(map[string]any{"code": 0, "req_0": map[string]any{"code": code, "data": data}})
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
				}
				if request.Req.Method == "get_song_detail_yqq" {
					details++
					return response(map[string]any{"track_info": map[string]any{"id": track.ID, "mid": track.MID, "file": map[string]any{"media_mid": track.MediaMID, "size_hires": 1000}}}, 0)
				}
				filename := request.Req.Param.Filename[0]
				prefix := filename[:4]
				requested = append(requested, prefix)
				if filename != prefix+track.MID+track.MediaMID+map[bool]string{true: ".flac", false: ".mp3"}[prefix == "RS01" || prefix == "F000"] {
					t.Fatal("incorrect playback filename")
				}
				if prefix == "RS01" {
					if request.Req.Module != "vkey.GetVkeyServer" || request.Req.Method != "CgiGetVkey" || request.Req.Param.Uin != "123" || request.Req.Param.Loginflag != 1 || request.Req.Param.Platform != "20" {
						t.Fatal("incorrect authenticated Hi-Res endpoint")
					}
					if mode == "permission" {
						return response(nil, 104003)
					}
				} else if request.Req.Method != "UrlGetVkey" {
					t.Fatal("changed ordinary-quality endpoint")
				}
				purl := filename + "?vkey=fixture"
				if prefix == "RS01" && (mode == "empty" || mode == "320k" || mode == "128k") || prefix == "F000" && (mode == "320k" || mode == "128k") || prefix == "M800" && mode == "128k" {
					purl = ""
				}
				return response(map[string]any{"sip": []string{"https://dl.stream.qqmusic.qq.com/"}, "midurlinfo": []any{map[string]any{"songmid": track.MID, "purl": purl}}}, 0)
			})}
			stream, err := SongURL(track.ID, "hires")
			if mode == "connection" {
				if err == nil || strings.Join(requested, ",") != "RS01" {
					t.Fatal("connection failure caused quality fallback")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want, size := "lossless", int64(800)
			switch mode {
			case "hires", "old queue":
				want, size = "hires", 123456
			case "320k":
				want, size = "exhigh", 600
			case "128k":
				want, size = "standard", 400
			}
			if stream.Level != want || stream.Size != size {
				t.Fatalf("level=%s size=%d, want %s/%d", stream.Level, stream.Size, want, size)
			}
			if mode == "old queue" && details != 1 {
				t.Fatal("old queue did not fetch Hi-Res metadata exactly once")
			}
		})
	}
}

func TestHiResTrackMetadataSurvivesQueueAndSparseSearch(t *testing.T) {
	track := convertTrack(obj(json.RawMessage(`{"id":8892,"mid":"metadataSong","file":{"media_mid":"media","size_hires":123456}}`)))
	b, _ := json.Marshal(track)
	var restored Track
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.SizeHiRes != 123456 {
		t.Fatal("queue lost Hi-Res size")
	}
	tracks := searchTracks([]object{obj(json.RawMessage(`{"id":8892,"mid":"metadataSong"}`))})
	if len(tracks) != 1 || tracks[0].SizeHiRes != 123456 {
		t.Fatal("sparse search lost Hi-Res size")
	}
}
