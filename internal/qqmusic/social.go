package qqmusic

import (
	"errors"
	"hash/fnv"
	"net/url"
	"strconv"
	"sync"
)

func PersonalFM(mode string) ([]Track, error) {
	if mode != FMDefault {
		return nil, ErrUnsupported
	}
	if sessionSnapshot().MusicKey == "" {
		return nil, errors.New("请先登录 QQ 音乐")
	}
	var data object
	if err := rpc("music.radioProxy.MbTrackRadioSvr", "get_radio_track", map[string]any{"id": 99, "firstplay": 1, "num": 15}, &data); err != nil {
		return nil, err
	}
	return convertTracks(objects(data, "tracks")), nil
}

// QQ requires the preceding page's opaque cursor, not a numeric offset.
var commentCursors = struct {
	sync.Mutex
	values map[string]map[int]string
}{values: map[string]map[int]string{}}

func ThreadComments(thread string, offset, limit int) (*Comments, error) {
	if _, err := strconv.ParseInt(thread, 10, 64); err != nil {
		return nil, err
	}
	limit = min(max(limit, 1), 50)
	cursor := ""
	if offset > 0 {
		commentCursors.Lock()
		cursor = commentCursors.values[thread][offset]
		commentCursors.Unlock()
		if cursor == "" {
			return nil, errors.New("QQ 评论分页游标已失效，请重新打开评论")
		}
	}
	var data object
	q := url.Values{"topid": {thread}, "pagenum": {strconv.Itoa(offset / limit)}, "pagesize": {strconv.Itoa(limit)}, "lasthotcommentid": {cursor}, "biztype": {"1"}, "cid": {"205360772"}, "cmd": {"8"}, "reqtype": {"2"}, "domain": {"qq.com"}, "ct": {"24"}, "cv": {"10101010"}, "needmusiccrit": {"0"}}
	if err := cgi("/base/fcgi-bin/fcg_global_comment_h5.fcg", q, &data); err != nil {
		return nil, err
	}
	latest, hot := obj(data["comment"]), obj(data["hot_comment"])
	if latest == nil && hot == nil {
		return nil, errors.New("QQ 音乐未返回评论数据")
	}
	result := &Comments{Total: int(number(latest, "commenttotal"))}
	decode := func(entries []object) []Comment {
		out := make([]Comment, 0, len(entries))
		for _, entry := range entries {
			remote := text(entry, "rootcommentid", "commentid")
			id := number(entry, "rootcommentid", "commentid")
			if id == 0 {
				h := fnv.New64a()
				_, _ = h.Write([]byte(remote))
				id = int64(h.Sum64() & 0x7fffffffffffffff)
			}
			c := Comment{ID: id, RemoteID: remote, Content: text(entry, "rootcommentcontent"), Time: number(entry, "time") * 1000, LikedCount: int(number(entry, "praisenum"))}
			c.User.Nickname = text(entry, "nick", "rootcommentnick")
			c.User.AvatarURL = text(entry, "avatarurl")
			out = append(out, c)
		}
		return out
	}
	entries := objects(latest, "commentlist")
	result.Latest = decode(entries)
	if offset == 0 {
		result.Hot = decode(objects(hot, "commentlist"))
	}
	if len(entries) > 0 {
		next := text(entries[len(entries)-1], "rootcommentid")
		result.More = next != "" && offset+len(entries) < result.Total
		if result.More {
			commentCursors.Lock()
			if commentCursors.values[thread] == nil {
				commentCursors.values[thread] = map[int]string{}
			}
			commentCursors.values[thread][offset+len(entries)] = next
			commentCursors.Unlock()
		}
	}
	return result, nil
}

func UserDetail(id int64) (*User, error) {
	p, err := Account()
	if err != nil {
		return nil, err
	}
	if p == nil || p.UserID != id {
		return nil, ErrUnsupported
	}
	return &User{UserID: p.UserID, Nickname: p.Nickname, AvatarURL: p.AvatarURL}, nil
}
