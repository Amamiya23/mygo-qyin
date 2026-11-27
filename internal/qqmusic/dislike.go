package qqmusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

func recommendationTracks(s Session, entries []object) []Track {
	tracks := convertTracks(entries)
	uid, _ := strconv.ParseInt(s.MusicID, 10, 64)
	for i := range tracks {
		tracks[i].RecommendationAccount = uid
	}
	return tracks
}

// ErrDislikeUnconfirmed means QQ may have applied the write, but its result is unknown.
var ErrDislikeUnconfirmed = errors.New("未能确认是否已加入 QQ 音乐黑名单")

const dislikeModule = "music.feedback.FeedbackBlack"

// DislikeSong adds one song to the specified signed-in account's blacklist.
// A successful write is confirmed by finding the song on the first page.
func DislikeSong(uid, id int64) error {
	if id <= 0 {
		return ErrUnsupported
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, err := dislikeSession(ctx, uid)
	if err != nil {
		return err
	}

	var write struct{ Retcode *int }
	param := map[string]any{"Songs": []map[string]string{{"ID": strconv.FormatInt(id, 10)}}}
	if err := rpcForSession(ctx, s, dislikeModule, "AddDislike", param, &write); err != nil {
		var protocol *ProtocolError
		if errors.As(err, &protocol) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrDislikeUnconfirmed, err)
	}
	if write.Retcode == nil {
		return ErrDislikeUnconfirmed
	}
	if *write.Retcode != 0 {
		return &ProtocolError{0, *write.Retcode, dislikeModule, "AddDislike"}
	}

	list, err := readDislikePage(ctx, s, 1, 0)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDislikeUnconfirmed, err)
	}
	for _, item := range list.Songs {
		if number(item, "ID") == id {
			return nil
		}
	}
	return ErrDislikeUnconfirmed
}

// SongBlacklist holds the song IDs QQ returned for the signed-in account.
// Absence is meaningful only when every page and the end were confirmed.
type SongBlacklist struct {
	IDs      map[int64]struct{}
	Complete bool
}

// ReadSongBlacklist only reads the account's song blacklist. On errors or
// ambiguous pagination it can still return confirmed IDs from the pages read.
func ReadSongBlacklist(uid int64) (SongBlacklist, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var result SongBlacklist
	s, err := dislikeSession(ctx, uid)
	if err != nil {
		return result, err
	}
	result.IDs = make(map[int64]struct{})
	var lastID int64
	verifiedPages := true
	const maxPages = 50
	for page := 1; page <= maxPages; page++ {
		list, err := readDislikePage(ctx, s, page, lastID)
		if err != nil {
			return result, err
		}
		actual := number(object{"Page": list.Page}, "Page")
		verifiedPages = verifiedPages && actual == int64(page)
		if len(list.Songs) == 0 {
			result.Complete = verifiedPages && list.Songs != nil
			return result, nil
		}
		var nextID int64
		added := 0
		for _, item := range list.Songs {
			nextID = number(item, "ID")
			if nextID <= 0 {
				verifiedPages = false
				continue
			}
			if _, found := result.IDs[nextID]; !found {
				result.IDs[nextID] = struct{}{}
				added++
			} else {
				verifiedPages = false
			}
		}
		if nextID <= 0 || nextID == lastID || added == 0 || (actual != 0 && actual != int64(page)) {
			return result, nil
		}
		lastID = nextID
	}
	return result, nil
}

type dislikePage struct {
	Retcode *int
	Page    json.RawMessage
	Songs   []object
}

func readDislikePage(ctx context.Context, s Session, page int, lastID int64) (dislikePage, error) {
	const method = "GetDislikeList"
	param := map[string]any{"Cmd": 3, "Page": page, "SongLastid": 0, "SingersLastid": 0}
	if lastID > 0 {
		param["SongLastid"] = strconv.FormatInt(lastID, 10)
	}
	var list dislikePage
	if err := rpcForSession(ctx, s, dislikeModule, method, param, &list); err != nil {
		return dislikePage{}, err
	}
	if list.Retcode == nil {
		return dislikePage{}, errors.New("qqmusic: missing blacklist status")
	}
	if *list.Retcode != 0 {
		return dislikePage{}, &ProtocolError{0, *list.Retcode, dislikeModule, method}
	}
	return list, nil
}

func dislikeSession(ctx context.Context, uid int64) (Session, error) {
	if uid <= 0 {
		return Session{}, ErrUnsupported
	}
	want := strconv.FormatInt(uid, 10)
	if sessionSnapshot().MusicID != want {
		return Session{}, ErrLoginRequired
	}
	s, err := activeSessionContext(ctx)
	if err != nil {
		return Session{}, err
	}
	if !s.valid() || s.MusicID != want {
		return Session{}, ErrLoginRequired
	}
	return s, nil
}
