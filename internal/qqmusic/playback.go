package qqmusic

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func trackForPlayback(id int64) (Track, error) {
	if t, ok := knownTrack(id); ok && t.MID != "" {
		return t, nil
	}
	tracks, err := SongDetails([]int64{id})
	if err != nil {
		return Track{}, err
	}
	if len(tracks) == 0 {
		return Track{}, ErrUnavailable
	}
	return tracks[0], nil
}

// SongURL asks QQ for a full stream at the requested quality, falling back to MP3.
func SongURL(id int64, level string) (*Stream, error) {
	t, err := trackForPlayback(id)
	if err != nil {
		return nil, err
	}
	type quality struct {
		prefix, extension, level string
		size                     int64
	}
	qualities := []quality{{"M500", "mp3", "standard", t.Size128}}
	if sessionSnapshot().MusicKey != "" {
		if Lossless(level) {
			qualities = append([]quality{{"F000", "flac", "lossless", t.SizeFLAC}, {"M800", "mp3", "exhigh", t.Size320}}, qualities...)
		} else if level == "higher" || level == "exhigh" {
			qualities = append([]quality{{"M800", "mp3", "exhigh", t.Size320}}, qualities...)
		}
	}
	media := t.MediaMID
	if media == "" {
		media = t.MID
	}
	guid := searchID()
	for _, q := range qualities {
		var data struct {
			SIP     []string `json:"sip"`
			Entries []struct {
				MID  string `json:"songmid"`
				PURL string `json:"purl"`
			} `json:"midurlinfo"`
		}
		s := sessionSnapshot()
		param := map[string]any{"filename": []string{q.prefix + t.MID + media + "." + q.extension}, "guid": guid, "songmid": []string{t.MID}, "songtype": []int{0}, "uin": s.MusicID}
		module, method := "music.vkey.GetVkey", "UrlGetVkey"
		if s.MusicKey == "" {
			module, method = "vkey.GetVkeyServer", "CgiGetVkey"
			param["uin"] = "0"
			param["loginflag"] = 1
			param["platform"] = "20"
		} else {
			param["ctx"] = 0
		}
		if err := rpc(module, method, param, &data); err != nil {
			return nil, err
		}
		for _, entry := range data.Entries {
			if entry.MID != "" && entry.MID != t.MID || entry.PURL == "" {
				continue
			}
			domain := ""
			for _, sip := range data.SIP {
				if validStreamDomain(sip) && !strings.HasPrefix(sip, "http://ws") {
					domain = sip
					break
				}
			}
			if domain == "" {
				for _, sip := range data.SIP {
					if validStreamDomain(sip) {
						domain = sip
						break
					}
				}
			}
			if domain == "" {
				domain = selectStreamDomain(guid, entry.PURL)
			}
			u, err := url.Parse(domain)
			if err != nil {
				return nil, err
			}
			p, err := url.Parse(entry.PURL)
			if err != nil {
				return nil, err
			}
			return &Stream{URL: u.ResolveReference(p).String(), Type: q.extension, Level: q.level, Size: q.size}, nil
		}
	}
	return nil, ErrUnavailable
}

func validStreamDomain(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && (u.Hostname() == "qqmusic.qq.com" || strings.HasSuffix(u.Hostname(), ".qqmusic.qq.com") || strings.HasSuffix(u.Hostname(), ".qq.com"))
}
func streamResponds(domain, purl string) bool {
	u, err := url.Parse(domain)
	if err != nil {
		return false
	}
	p, err := url.Parse(purl)
	if err != nil {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, u.ResolveReference(p).String(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", UserAgent)
	res, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK || res.StatusCode == http.StatusPartialContent
}
func selectStreamDomain(guid, purl string) string {
	const preferred = "http://sjy6.stream.qqmusic.qq.com/"
	if streamResponds(preferred, purl) {
		return preferred
	}
	var data struct {
		SIP []string `json:"sip"`
	}
	if rpc("music.audioCdnDispatch.cdnDispatch", "GetCdnDispatch", map[string]any{"guid": guid, "uid": "0", "use_new_domain": 1, "use_ipv6": 1}, &data) == nil {
		for _, sip := range data.SIP {
			if validStreamDomain(sip) && streamResponds(sip, purl) {
				return sip
			}
		}
	}
	return "http://dl.stream.qqmusic.qq.com/"
}

func Lyrics(id int64) (lrc, translation, yrc string, err error) {
	t, err := trackForPlayback(id)
	if err != nil {
		return "", "", "", err
	}
	var data struct {
		Lyric       string `json:"lyric"`
		Translation string `json:"trans"`
	}
	err = cgi("/lyric/fcgi-bin/fcg_query_lyric_new.fcg", url.Values{"songmid": {t.MID}, "pcachetime": {strconv.FormatInt(int64(id), 10)}}, &data)
	if err != nil {
		return "", "", "", err
	}
	decode := func(s string) (string, error) {
		if s == "" {
			return "", nil
		}
		b, e := base64.StdEncoding.DecodeString(s)
		if e != nil {
			return "", fmt.Errorf("qqmusic: malformed lyric encoding: %w", e)
		}
		return string(b), nil
	}
	lrc, err = decode(data.Lyric)
	if err != nil {
		return "", "", "", err
	}
	translation, err = decode(data.Translation)
	return lrc, translation, "", err
}
