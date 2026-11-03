package qqmusic

import (
	"fmt"
	"strconv"
)

// DislikeType identifies the kind of item in QQ Music's feedback blacklist.
type DislikeType int

const (
	DislikeSong   DislikeType = 1
	DislikeSinger DislikeType = 2
)

// AddDislike submits a song or singer's numeric ID to the account's feedback list.
func AddDislike(kind DislikeType, id int64) error {
	if _, err := signedSession(); err != nil {
		return err
	}
	key := ""
	switch kind {
	case DislikeSong:
		key = "Songs"
	case DislikeSinger:
		key = "Singers"
	default:
		return fmt.Errorf("qqmusic: invalid dislike type")
	}
	if id <= 0 {
		return fmt.Errorf("qqmusic: invalid dislike ID")
	}
	param := map[string]any{key: []map[string]any{{"ID": strconv.FormatInt(id, 10), "IdType": kind}}}
	var data struct {
		Retcode *int `json:"Retcode"`
	}
	const module, method = "music.feedback.FeedbackBlack", "AddDislike"
	if err := rpc(module, method, param, &data); err != nil {
		return err
	}
	if data.Retcode == nil {
		return fmt.Errorf("qqmusic: missing dislike status")
	}
	if *data.Retcode != 0 {
		return &ProtocolError{0, *data.Retcode, module, method}
	}
	return nil
}
