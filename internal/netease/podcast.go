package netease

import (
	"fmt"
	"net/url"
	"strconv"
)

// Podcast is a channel of episodes, which NetEase calls a radio.
type Podcast struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	PicURL       string `json:"picUrl"`
	Description  string `json:"desc"`
	Category     string `json:"category"`
	DJ           User   `json:"dj"`
	ProgramCount int    `json:"programCount"`
	SubCount     int64  `json:"subCount"`
	Subed        bool   `json:"subed"` // by the listener signed in
}

// Program is an episode of a podcast.
type Program struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	CoverURL    string      `json:"coverUrl"`
	Duration    int64       `json:"duration"`   // milliseconds
	CreateTime  int64       `json:"createTime"` // Unix milliseconds
	Description string      `json:"description"`
	MainSong    legacyTrack `json:"mainSong"` // its audio
}

// Track is the episode as it plays, in a podcast: its audio's ID, which
// streams, under the episode's name, cover and host.
func (p *Program) Track(pod *Podcast) Track {
	cover := p.CoverURL
	if cover == "" {
		cover = pod.PicURL
	}
	t := Track{
		ID: p.MainSong.ID, Name: p.Name, Duration: p.Duration,
		Album:     Album{Name: pod.Name, PicURL: cover},
		ProgramID: p.ID, PodcastID: pod.ID, Description: p.Description,
	}
	if pod.DJ.Nickname != "" {
		t.Artists = []Artist{{Name: pod.DJ.Nickname}}
	}
	return t
}

// PodcastDetail returns a podcast.
func PodcastDetail(id int64) (*Podcast, error) {
	var res struct {
		Data *Podcast `json:"data"`
	}
	if err := get("/api/djradio/v2/get", url.Values{"id": {strconv.FormatInt(id, 10)}}, &res); err != nil {
		return nil, err
	}
	if res.Data == nil || res.Data.ID != id {
		return nil, fmt.Errorf("netease: no podcast %d", id)
	}
	return res.Data, nil
}

// PodcastPrograms returns limit of a podcast's episodes from offset, the
// newest first, and whether it has more.
func PodcastPrograms(id int64, limit, offset int) ([]Program, bool, error) {
	var res struct {
		Programs []Program `json:"programs"`
		More     bool      `json:"more"`
	}
	q := url.Values{"radioId": {strconv.FormatInt(id, 10)}, "limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}, "asc": {"false"}}
	err := get("/api/dj/program/byradio", q, &res)
	return res.Programs, res.More, err
}
