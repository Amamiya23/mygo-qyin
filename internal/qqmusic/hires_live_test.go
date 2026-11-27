package qqmusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

// TestLiveHiRes probes explicit formats without SongURL's quality fallback.
// It never refreshes or writes the saved credential, or logs signed URLs.
func TestLiveHiRes(t *testing.T) {
	if os.Getenv("LIVE") != "1" || os.Getenv("LIVE_HIRES_ACCOUNT") != "1" {
		t.Skip("see docs/hires-probe.md for the opt-in account probe")
	}
	mid := os.Getenv("LIVE_HIRES_SONG_MID")
	if !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(mid) {
		t.Fatal("set LIVE_HIRES_SONG_MID to the MID from a QQ Music song link")
	}
	var b []byte
	var err error
	if file := os.Getenv("LIVE_HIRES_SESSION_FILE"); file != "" {
		b, err = os.ReadFile(file)
	} else {
		b, err = readInstalledSession()
	}
	if err != nil {
		t.Fatal("cannot read saved login; see docs/hires-probe.md")
	}
	var s Session
	if json.Unmarshal(b, &s) != nil || !s.valid() {
		t.Fatal("saved login is invalid or expired; sign in to Lilt again")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(s)
	t.Logf("loginType=%d (1=WeChat, 2=QQ)", s.LoginType)

	// Supplying comm explicitly avoids automatic refresh and persistence.
	var detail object
	err = rpcWithComm("music.pf_song_detail_svr", "get_song_detail_yqq", map[string]any{
		"song_type": 0, "song_mid": mid, "song_id": "0",
	}, &detail, authenticatedComm(s), nil)
	if err != nil {
		t.Fatal("song detail: ", hiResProbeError(err))
	}
	track := obj(detail["track_info"])
	file := obj(track["file"])
	media := text(file, "media_mid")
	if text(track, "mid") != mid || media == "" {
		t.Fatal("song detail did not return matching MID and media_mid")
	}
	t.Logf("song ID=%d", number(track, "id"))
	t.Logf("file sizes: size_hires=%d size_flac=%d (metadata alone does not prove access)", number(file, "size_hires"), number(file, "size_flac"))
	verified := false
	for _, endpoint := range []struct{ module, method string }{
		{"music.vkey.GetVkey", "UrlGetVkey"},
		{"vkey.GetVkeyServer", "CgiGetVkey"},
	} {
		for _, prefix := range []string{"RS01", "F000"} {
			for _, naming := range []struct{ label, stem string }{
				{"song+media", mid + media}, // Current app convention.
				{"media", media},
			} {
				label := endpoint.method + "/" + prefix + "/" + naming.label
				param := map[string]any{
					"filename": []string{prefix + naming.stem + ".flac"},
					"guid":     searchID(), "songmid": []string{mid}, "songtype": []int{0}, "uin": s.MusicID,
				}
				if endpoint.method == "UrlGetVkey" {
					param["ctx"] = 0
				} else {
					param["loginflag"], param["platform"] = 1, "20"
				}
				var data struct {
					SIP     []string `json:"sip"`
					Entries []struct {
						MID    string `json:"songmid"`
						PURL   string `json:"purl"`
						Result int    `json:"result"`
					} `json:"midurlinfo"`
				}
				if err := rpcWithComm(endpoint.module, endpoint.method, param, &data, authenticatedComm(s), nil); err != nil {
					t.Logf("%s: %s", label, hiResProbeError(err))
					continue
				}
				if len(data.Entries) == 0 {
					t.Logf("%s: no midurlinfo entries", label)
				}
				for _, entry := range data.Entries {
					if entry.MID != "" && entry.MID != mid {
						continue
					}
					if entry.PURL == "" {
						t.Logf("%s: empty purl, result=%d", label, entry.Result)
						continue
					}
					p, err := url.Parse(entry.PURL)
					if err != nil || p.IsAbs() || p.Host != "" || !strings.HasPrefix(path.Base(p.Path), prefix) {
						t.Logf("%s: unexpected returned path or format", label)
						continue
					}
					for _, domain := range data.SIP {
						if !validStreamDomain(domain) {
							continue
						}
						u, _ := url.Parse(domain)
						info, _, err := inspectFLACStream(context.Background(), u.ResolveReference(p).String())
						if err != nil {
							t.Logf("%s: %s", label, hiResProbeError(err))
							continue
						}
						t.Logf("%s: FLAC %d-bit %d Hz channels=%d", label, info.BitsPerSample, info.SampleRate, info.NChannels)
						if prefix == "RS01" && info.BitsPerSample >= 24 && info.SampleRate >= 44100 {
							verified = true
						}
						break
					}
				}
			}
		}
	}
	if !verified {
		t.Fatal("no RS01 FLAC header with >=24-bit / >=44100 Hz verified; inspect the diagnostics or try another known Hi-Res song")
	}
	t.Log("verified RS01 delivery and high-resolution FLAC metadata; full playback was not tested")
}

func hiResProbeError(err error) string {
	var protocol *ProtocolError
	if errors.As(err, &protocol) {
		return protocol.Error()
	}
	var status loginHTTPError
	if errors.As(err, &status) {
		return fmt.Sprintf("CDN HTTP %d", status)
	}
	// URL errors can contain signed media URLs. Do not print their messages.
	return fmt.Sprintf("request or FLAC header failed (%T)", err)
}
