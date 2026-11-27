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
	"testing/synctest"
	"time"
)

func TestHiResPlaybackAndFallback(t *testing.T) {
	for _, mode := range []string{"hires", "permission", "empty", "403", "404", "HTML", "low resolution", "320k", "128k", "connection", "old queue", "cached lossless", "cached 320k"} {
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
				switch {
				case prefix == "RS01" && (mode == "empty" || mode == "320k" || mode == "128k" || strings.HasPrefix(mode, "cached ")),
					prefix == "F000" && (mode == "320k" || mode == "128k"),
					prefix == "M800" && mode == "128k":
					purl = ""
				}
				return response(map[string]any{"sip": []string{"https://dl.stream.qqmusic.qq.com/"}, "midurlinfo": []any{map[string]any{"songmid": track.MID, "purl": purl}}}, 0)
			})}
			var stream *Stream
			var err error
			switch mode {
			case "cached lossless":
				stream, err = SongURLAbove(context.Background(), track.ID, "hires", "lossless")
				if stream != nil || !errors.Is(err, ErrUnavailable) || strings.Join(requested, ",") != "RS01" {
					t.Fatalf("lookup did not stop at the complete cache: requested=%v, err=%v", requested, err)
				}
				return
			case "cached 320k":
				stream, err = SongURLAbove(context.Background(), track.ID, "hires", "exhigh")
				if strings.Join(requested, ",") != "RS01,F000" {
					t.Fatalf("lookup did not try every quality above the cached 320k: %v", requested)
				}
			default:
				stream, err = SongURL(track.ID, "hires")
			}
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

func TestHiResLookupCancelsInFlightRequests(t *testing.T) {
	for _, stage := range []string{"details", "playback address", "FLAC probe"} {
		t.Run(stage, func(t *testing.T) {
			previous, prior := httpClient, sessionSnapshot()
			t.Cleanup(func() { httpClient = previous; setSession(prior) })
			setSession(Session{MusicID: "123", MusicKey: "fixture-key", LoginType: 1})
			track := Track{ID: 8893, MID: "cancelFixture", SizeHiRes: 1000, NeedsDetails: stage == "details"}
			RememberTracks([]Track{track})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			httpClient = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
				if stage == "FLAC probe" && r.Method == http.MethodPost {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"req_0":{"code":0,"data":{"sip":["https://dl.stream.qqmusic.qq.com/"],"midurlinfo":[{"purl":"RS01fixture.flac"}]}}}`))}, nil
				}
				cancel()
				if !errors.Is(r.Context().Err(), context.Canceled) {
					t.Fatal("request did not inherit the playback context")
				}
				return nil, r.Context().Err()
			})}
			if _, err := SongURLAbove(ctx, track.ID, "hires", "lossless"); !errors.Is(err, context.Canceled) {
				t.Fatalf("lookup error = %v, want cancellation", err)
			}
		})
	}
}

func TestHiResLookupDeadlineKeepsRecoverableSession(t *testing.T) {
	previous, prior := authHTTP, sessionSnapshot()
	t.Cleanup(func() { authHTTP = previous; setSession(prior) })
	expired := Session{MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, OpenID: "open", RefreshToken: "token", Device: &wechatDevice{}}
	setSession(expired)
	track := Track{ID: 8894, MID: "deadlineFixture", SizeHiRes: 1000}
	RememberTracks([]Track{track})
	authHTTP = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := SongURLAbove(ctx, track.ID, "hires", "lossless"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("lookup error = %v, want deadline exceeded", err)
		}
		if sessionSnapshot() != expired || sessionRefreshError() != nil {
			t.Fatal("playback deadline left the account unable to renew later")
		}
	})
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
			info, _, err := inspectFLACStream(context.Background(), "https://dl.stream.qqmusic.qq.com/RS01test.flac?vkey=private")
			if !tc.ok {
				if err == nil {
					t.Fatal("accepted unavailable or invalid audio")
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
