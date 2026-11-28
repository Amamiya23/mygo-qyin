package qqmusic

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Canceling a playback lookup also cancels the HTTP request resolving its URL.
func TestSongURLContextCancelsRequest(t *testing.T) {
	previous, prior := httpClient, sessionSnapshot()
	t.Cleanup(func() { httpClient = previous; setSession(prior) })
	setSession(Session{})
	track := Track{ID: 8892, MID: "cancelSong"}
	RememberTracks([]Track{track})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	httpClient = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
		cancel()
		if err := r.Context().Err(); err != context.Canceled {
			t.Fatalf("playback HTTP request was not canceled: %v", err)
		}
		return nil, r.Context().Err()
	})}
	if _, err := SongURLContext(ctx, track.ID, "standard"); !errors.Is(err, context.Canceled) {
		t.Fatalf("playback lookup returned %v, want context cancellation", err)
	}
}

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

func TestHiResProbeHeader(t *testing.T) {
	header := hiResTestHeader()
	for _, tc := range []struct {
		name   string
		body   []byte
		status int
		ok     bool
	}{
		{"FLAC", header, 206, true},
		{"server ignores Range", header, 200, true},
		{"ID3 prefix", append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}, header...), 206, true},
		{"missing resource", header, 404, false},
		{"server error falls back", header, 502, false},
		{"HTML response", []byte("<html>not audio</html>"), 200, false},
		{"truncated header", header[:30], 206, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := httpClient
			t.Cleanup(func() { httpClient = previous })
			httpClient = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Range") != "bytes=0-65535" {
					t.Error("missing bounded Range request")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(bytes.NewReader(tc.body))}, nil
			})}
			info, _, err := inspectFLACStream(t.Context(), "https://dl.stream.qqmusic.qq.com/RS01test.flac?vkey=private")
			if !tc.ok {
				if err == nil {
					t.Fatal("accepted unavailable or invalid audio")
				}
				if tc.status == 502 && !errors.Is(err, ErrUnavailable) {
					t.Fatalf("HTTP 502 returned %v, want ErrUnavailable for quality fallback", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if info.BitsPerSample != 24 || info.SampleRate != 96000 || info.NChannels != 2 {
				t.Fatalf("incorrect audio metadata: %+v", info)
			}
		})
	}
}

func TestHiResProbeErrorRedactsURL(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://example.com/?vkey=private", Err: errors.New("private")}
	if message := hiResProbeError(err); strings.Contains(message, "private") || strings.Contains(message, "example.com") {
		t.Fatal("diagnostics leaked signed URL")
	}
}

// Stream domain validation normalizes host casing and rejects userinfo or non-QQ hosts.
func TestValidStreamDomainHardening(t *testing.T) {
	if !validStreamDomain("https://DL.STREAM.QQMUSIC.QQ.COM/song.flac") {
		t.Fatal("rejected valid uppercase QQ stream host")
	}
	for _, raw := range []string{
		"https://user:pass@dl.stream.qqmusic.qq.com/song.flac",
		"https://qqmusic.qq.com@evil.example/song.flac",
		"//dl.stream.qqmusic.qq.com/song.flac",
	} {
		if validStreamDomain(raw) {
			t.Fatalf("accepted unsafe stream domain %q", raw)
		}
	}
}

func hiResTestHeader() []byte {
	// Minimal STREAMINFO: 24-bit, 96 kHz, stereo. No other metadata or audio
	// frames are needed to identify the format returned by the CDN.
	header := make([]byte, 42)
	copy(header, "fLaC")
	header[4], header[7] = 0x80, 34
	binary.BigEndian.PutUint16(header[8:10], 4096)
	binary.BigEndian.PutUint16(header[10:12], 4096)
	binary.BigEndian.PutUint64(header[18:26], uint64(96000)<<44|uint64(1)<<41|uint64(23)<<36|96000)
	return header
}
