package app

import (
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

var addDislike = qqmusic.AddDislike

// dailySongItems keeps feedback specific to the daily recommendation context.
func (a *app) dailySongItems(tr *qqmusic.Track) func(*ui.Menu) {
	return func(m *ui.Menu) {
		a.songItems(tr, 0)(m)
		if tr.ID <= 0 {
			return
		}
		m.Separator()
		if m.Item("不喜欢这首歌曲").Chosen() {
			a.dislike(qqmusic.DislikeSong, tr.ID)
		}
		seen := map[int64]bool{}
		for _, artist := range tr.Artists {
			if artist.ID <= 0 || artist.Name == "" || seen[artist.ID] {
				continue
			}
			seen[artist.ID] = true
			if m.Item("不喜欢歌手：" + artist.Name).Chosen() {
				a.dislike(qqmusic.DislikeSinger, artist.ID)
			}
		}
	}
}

func (a *app) dislike(kind qqmusic.DislikeType, id int64) {
	if a.profile == nil {
		a.showMain()
		a.openLogin()
		return
	}
	uid := a.profile.UserID
	go func() {
		err := addDislike(kind, id)
		a.post(func() {
			if a.profile == nil || a.profile.UserID != uid {
				return
			}
			if err != nil {
				a.toast = "提交不喜欢失败，请稍后再试"
			} else {
				a.toast = "已提交不喜欢"
			}
		})
	}()
}
