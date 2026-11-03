package qqmusic

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"lilt/internal/lyrics"
)

// The old endpoint often omits translations; a failed supplement must still leave the original readable.
func TestLyricsTranslationFallback(t *testing.T) {
	const original = "[00:12.50]A foreign lyric\n[00:18.00]Another line"
	const translated = "[00:12.50]一句译文\n[00:18.00]另一句译文"
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	for _, tc := range []struct {
		name, existing, fallback, want string
		status                         int
	}{
		{name: "missing", fallback: encode(translated), want: translated},
		{name: "existing", existing: encode(translated), want: translated},
		{name: "unavailable"},
		{name: "encrypted", fallback: "ABCDEF0123456789"},
		{name: "malformed base64", fallback: "not-base64!"},
		{name: "untimed", fallback: encode("没有时间戳")},
		{name: "rejected", fallback: encode(translated), status: 2001},
		{name: "HTTP failure", status: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RememberTracks([]Track{{ID: 994, MID: "translatedSong"}})
			oldCalls, newCalls, wordCalls := 0, 0, 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/lyric/fcgi-bin/fcg_query_lyric_new.fcg" {
					oldCalls++
					if r.URL.Query().Get("songmid") != "translatedSong" {
						t.Error("lyric request lost song MID")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "lyric": encode(original), "trans": tc.existing})
					return
				}
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          map[string]any
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.Req.Param["crypt"] == float64(1) {
					wordCalls++
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 24001}})
					return
				}
				newCalls++
				if request.Req.Module != "music.musichallSong.PlayLyricInfo" || request.Req.Method != "GetPlayLyricInfo" || request.Req.Param["songMID"] != "translatedSong" || request.Req.Param["crypt"] != float64(0) || request.Req.Param["trans"] != float64(1) {
					t.Errorf("wrong translation request: %+v", request.Req)
				}
				if request.Comm["tmeAppID"] != "qqmusiclight" || request.Comm["uid"] != "0" || request.Comm["authst"] != nil || r.Header.Get("Cookie") != "tmeLoginType=-1" || r.UserAgent() != "okhttp/3.14.9" {
					t.Error("public lyric translation inherited account identity")
				}
				if tc.status == http.StatusBadGateway {
					w.WriteHeader(tc.status)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": tc.status, "data": map[string]any{"lyric": "encrypted-original", "trans": tc.fallback}}})
			})
			setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2})
			lrc, trans, yrc, err := Lyrics(994)
			if err != nil || lrc != original || trans != tc.want || yrc != "" {
				t.Fatalf("lyrics=%q translation=%q yrc=%q error=%v", lrc, trans, yrc, err)
			}
			wantCalls := 1
			if tc.existing != "" {
				wantCalls = 0
			}
			if oldCalls != 1 || newCalls != wantCalls || wordCalls != 1 {
				t.Fatalf("requests: old=%d translation=%d word=%d, want 1/%d/1", oldCalls, newCalls, wordCalls, wantCalls)
			}
			lines := lyrics.Build(lrc, trans, yrc)
			wantLine := ""
			if tc.want != "" {
				wantLine = "一句译文"
			}
			if len(lines) != 2 || lines[0].Translation != wantLine {
				t.Fatalf("translation did not reach timed lines: %+v", lines)
			}
		})
	}
}

// Real QRC must reach the player without estimating word durations, using the
// account identity when signed in and the anonymous browser identity when not.
func TestLyricsRealWordTimes(t *testing.T) {
	data, err := os.ReadFile("testdata/qrc_qingtian.hex")
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []struct {
		name    string
		session Session
	}{
		{name: "anonymous"},
		{name: "authenticated", session: Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2}},
	} {
		for _, version := range []int{0, 1761231326} {
			t.Run(identity.name+"/version="+fmt.Sprint(version), func(t *testing.T) {
				RememberTracks([]Track{{ID: 97773, MID: "0039MnYb0qxYhV"}})
				calls := 0
				withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.URL.Path != "/cgi-bin/musicu.fcg" {
						t.Errorf("word lyrics fell back to %s", r.URL.Path)
					}
					var request struct {
						Comm map[string]any
						Req  struct {
							Module, Method string
							Param          map[string]any
						} `json:"req_0"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					if request.Req.Module != "music.musichallSong.PlayLyricInfo" || request.Req.Method != "GetPlayLyricInfo" || request.Req.Param["songMID"] != "0039MnYb0qxYhV" || request.Req.Param["crypt"] != float64(1) || request.Req.Param["qrc"] != float64(1) || request.Req.Param["trans"] != float64(1) {
						t.Errorf("wrong QRC request: %+v", request.Req)
					}
					if identity.session.MusicKey == "" {
						if request.Comm["uin"] != "0" || request.Comm["ct"] != float64(24) || request.Comm["authst"] != nil || r.Header.Get("Cookie") != "" || r.UserAgent() != "Mozilla/5.0" || r.Header.Get("Referer") != "https://y.qq.com/" {
							t.Error("signed-out QRC request lost anonymous browser identity")
						}
					} else if request.Comm["qq"] != identity.session.MusicID || request.Comm["authst"] != identity.session.MusicKey || request.Comm["tmeLoginType"] != float64(identity.session.LoginType) || !strings.Contains(r.Header.Get("Cookie"), "qm_keyst="+identity.session.MusicKey) {
						t.Errorf("signed-in QRC request lost account identity: %+v", request.Comm)
					}
					rpcReply(w, map[string]any{"lyric": strings.TrimSpace(string(data)), "qrc_t": version, "trans": ""})
				})
				setSession(identity.session)
				t.Cleanup(func() { setSession(Session{}) })
				lrc, trans, qrc, err := Lyrics(97773)
				if err != nil || lrc != "" || trans != "" || qrc == "" || calls != 1 {
					t.Fatalf("QRC request failed: lrc=%q trans=%q qrc bytes=%d calls=%d err=%v", lrc, trans, len(qrc), calls, err)
				}
				lines := lyrics.Build(lrc, trans, qrc)
				for _, line := range lines {
					if line.At == 29264*time.Millisecond {
						if !line.Timed || len(line.Words) != 6 || line.Words[0].Start != 29264*time.Millisecond || line.Words[0].End != 29654*time.Millisecond || line.Words[3].End-line.Words[3].Start != 922*time.Millisecond {
							t.Fatalf("real word durations were changed: %+v", line)
						}
						return
					}
				}
				t.Fatal("real sung line missing from QRC")
			})
		}
	}
}

// Encrypted LRC stays line-timed, while encrypted translations remain attached to real QRC.
func TestLyricsEncryptedPayloadKinds(t *testing.T) {
	for _, tc := range []struct {
		name, original, translation string
		timed                       bool
	}{
		{name: "LRC only", original: "qrc_lrc_only.hex"},
		{name: "QRC with translation", original: "qrc_kimi.hex", translation: "qrc_kimi_trans.hex", timed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := func(name string) string {
				if name == "" {
					return ""
				}
				data, err := os.ReadFile("testdata/" + name)
				if err != nil {
					t.Fatal(err)
				}
				return strings.TrimSpace(string(data))
			}
			original, translation := read(tc.original), read(tc.translation)
			wantOriginal, err := decodeQRC(original)
			if err != nil {
				t.Fatal(err)
			}
			wantTranslation, err := decodeQRC(translation)
			if err != nil {
				t.Fatal(err)
			}
			RememberTracks([]Track{{ID: 996, MID: "payloadKindSong"}})
			calls := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/cgi-bin/musicu.fcg" {
					t.Errorf("readable encrypted lyrics fell back to %s", r.URL.Path)
				}
				rpcReply(w, map[string]any{"lyric": original, "trans": translation, "qrc_t": 1761231326})
			})
			lrc, trans, qrc, err := Lyrics(996)
			if err != nil || trans != wantTranslation || calls != 1 {
				t.Fatalf("encrypted response lost: trans=%q calls=%d error=%v", trans, calls, err)
			}
			if tc.timed && (qrc != wantOriginal || lrc != "") || !tc.timed && (lrc != wantOriginal || qrc != "") {
				t.Fatalf("wrong lyric kind: timed=%v lrc bytes=%d qrc bytes=%d", tc.timed, len(lrc), len(qrc))
			}
			lines := lyrics.Build(lrc, trans, qrc)
			translations := 0
			for _, line := range lines {
				if line.Timed != tc.timed {
					t.Fatalf("wrong timing flag: %+v", line)
				}
				if line.Translation != "" {
					translations++
				}
			}
			if tc.translation != "" && translations == 0 {
				t.Fatal("decoded translation never reached timed lines")
			}
		})
	}
}

// Unavailable or malformed QRC must leave ordinary LRC and its translation usable.
func TestLyricsWordEndpointFallback(t *testing.T) {
	const original = "[00:12.50]A lyric\n[00:18.00]Another line"
	const translated = "[00:12.50]译文"
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	for _, tc := range []struct {
		name, cipher string
		status       int
	}{
		{name: "empty"},
		{name: "invalid hex", cipher: "not-hex"},
		{name: "invalid ciphertext", cipher: "0000000000000000"},
		{name: "rejected", status: 24001},
		{name: "HTTP failure", status: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RememberTracks([]Track{{ID: 995, MID: "fallbackSong"}})
			wordCalls, oldCalls := 0, 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/lyric/fcgi-bin/fcg_query_lyric_new.fcg" {
					oldCalls++
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "lyric": encode(original), "trans": encode(translated)})
					return
				}
				wordCalls++
				if tc.status == http.StatusBadGateway {
					w.WriteHeader(tc.status)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": tc.status, "data": map[string]any{"lyric": tc.cipher, "qrc_t": 1761231326}}})
			})
			lrc, trans, qrc, err := Lyrics(995)
			if err != nil || lrc != original || trans != translated || qrc != "" || wordCalls != 1 || oldCalls != 1 {
				t.Fatalf("fallback failed: lrc=%q trans=%q qrc=%q calls=%d/%d error=%v", lrc, trans, qrc, wordCalls, oldCalls, err)
			}
		})
	}
}

// A broken translation must not replace successfully decoded real word timings with estimated LRC.
func TestLyricsQRCTranslationFailure(t *testing.T) {
	data, err := os.ReadFile("testdata/qrc_qingtian.hex")
	if err != nil {
		t.Fatal(err)
	}
	const translated = "[00:29.264]译文"
	RememberTracks([]Track{{ID: 97773, MID: "0039MnYb0qxYhV"}})
	wordCalls, translationCalls := 0, 0
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cgi-bin/musicu.fcg" {
			t.Errorf("translation failure discarded QRC: %s", r.URL.Path)
		}
		var request struct {
			Req struct{ Param map[string]any } `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Req.Param["crypt"] == float64(1) {
			wordCalls++
			rpcReply(w, map[string]any{"lyric": strings.TrimSpace(string(data)), "trans": "corrupt"})
		} else {
			translationCalls++
			rpcReply(w, map[string]any{"trans": base64.StdEncoding.EncodeToString([]byte(translated))})
		}
	})
	lrc, trans, qrc, err := Lyrics(97773)
	if err != nil || lrc != "" || trans != translated || qrc == "" || wordCalls != 1 || translationCalls != 1 {
		t.Fatalf("translation lost original QRC: lrc=%q trans=%q qrc bytes=%d calls=%d/%d error=%v", lrc, trans, len(qrc), wordCalls, translationCalls, err)
	}
}

// A restored queue must retain MIDs and the chosen file size so seeking need not wait for EOF.
func TestPlaybackRestoredSizeAndQualityFallback(t *testing.T) {
	track := convertTrack(obj(json.RawMessage(`{"id":993,"mid":"restoredSong","title":"Song","file":{"media_mid":"restoredMedia","size_128mp3":123456,"size_320mp3":345678,"size_flac":987654},"volume":{"gain":-7.603,"peak":0.946,"lra":3.712}}`)))
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
			if stream.Level != served.level || stream.Size != served.size || stream.Gain != -7.603 || stream.Peak != 0.946 {
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
