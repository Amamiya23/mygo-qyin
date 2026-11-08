package qqmusic

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// SongFansNumberContext returns QQ Music's display text for a song's fan count.
func SongFansNumberContext(ctx context.Context, id int64) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("qqmusic: 无效的歌曲 ID")
	}
	var data struct {
		Show map[string]string `json:"m_show"`
	}
	if err := rpcWithCommContext(ctx, "music.musicasset.SongFavRead", "GetSongFansNumberById",
		map[string]any{"v_songId": []int64{id}}, &data, nil, nil); err != nil {
		return "", err
	}
	show := strings.TrimSpace(data.Show[strconv.FormatInt(id, 10)])
	if show == "" {
		return "", fmt.Errorf("qqmusic: 未返回歌曲喜欢人数")
	}
	return show, nil
}
