package app

import (
	"errors"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

type dislikeStatus uint8

const (
	dislikePending dislikeStatus = iota + 1
	dislikeConfirmed
)

type songDislikes struct {
	profile *qqmusic.Profile
	songs   map[int64]dislikeStatus
}

type dailyDislikes struct {
	profile  *qqmusic.Profile
	ids      map[int64]struct{}
	loading  bool
	complete bool
	writes   uint64 // confirmed clicks since this profile was activated
}

var dislikeSong = qqmusic.DislikeSong
var readSongBlacklist = qqmusic.ReadSongBlacklist

func (a *app) loadDailyDislikes() {
	prof := a.profile
	if prof == nil || prof.UserID <= 0 {
		return
	}
	d := a.dailyDislikes
	if d == nil || d.profile != prof {
		d = &dailyDislikes{profile: prof, ids: make(map[int64]struct{})}
		a.dailyDislikes = d
	}
	if d.loading {
		return
	}
	d.loading = true
	uid, writes, read := prof.UserID, d.writes, readSongBlacklist
	go func() {
		list, err := read(uid)
		a.post(func() {
			if a.profile != prof || a.dailyDislikes != d {
				return
			}
			d.loading = false
			if err == nil && list.Complete && writes == d.writes {
				d.ids, d.complete = list.IDs, true
				return
			}
			// Partial results only confirm membership; an earlier read also
			// cannot clear a dislike confirmed while that read was in flight.
			for id := range list.IDs {
				d.record(id)
			}
		})
	}()
}

func (d *dailyDislikes) record(id int64) {
	if d.ids == nil {
		d.ids = make(map[int64]struct{})
	}
	d.ids[id] = struct{}{}
}

func (a *app) canDislike(tr *qqmusic.Track) bool {
	return a.profile != nil && tr != nil && tr.ID > 0 && tr.ProgramID == 0 &&
		tr.RecommendationAccount > 0 && tr.RecommendationAccount == a.profile.UserID
}

func (a *app) dislikeState(tr *qqmusic.Track) dislikeStatus {
	if tr == nil || tr.ProgramID != 0 {
		return 0
	}
	var local dislikeStatus
	if d := a.dislikes; d != nil && d.profile == a.profile {
		local = d.songs[tr.ID]
	}
	if local != dislikePending && tr.FromDailyRecommendations && a.canDislike(tr) {
		if d := a.dailyDislikes; d != nil && d.profile == a.profile {
			if _, found := d.ids[tr.ID]; found {
				return dislikeConfirmed
			}
			if d.complete {
				return 0
			}
		}
	}
	return local
}

func (a *app) skipRecommendation(tr *qqmusic.Track) bool {
	return a.canDislike(tr) && a.dislikeState(tr) == dislikeConfirmed
}

const dislikedLabel = "已不喜欢"

func recommendationRowLabel(name string, skipped bool) string {
	if skipped {
		return name + "（" + dislikedLabel + "）"
	}
	return name
}

func dislikedBadge(c *ui.Context, color ui.Color) {
	ui.Text(c, dislikedLabel).FontSize(10.5).TextColor(color).SingleLine().Shrink(0)
}

func (a *app) dislikeButton(c *ui.Context, tr *qqmusic.Track, size float32, color ui.Color) {
	if !a.canDislike(tr) {
		return
	}
	status := a.dislikeState(tr)
	if status == dislikeConfirmed {
		// Show the outcome in the same slot, without the hover and press effects of an action.
		roundButtonBase(c, size, color, dislikedLabel).Shrink(0).Disabled(true).DimDisabled(false).
			Children(func() { ui.Icon(c, iconHeartOffFill).Size(size, size) })
		return
	}
	label := "不喜欢这首歌"
	if status == dislikePending {
		label = "正在提交不喜欢"
	}
	iconButton(c, iconHeartOff, size, color, label, mediaSurface).Shrink(0).
		Disabled(status != 0 || !a.nowPlaying).
		OnClick(func() { a.dislikeCurrent(tr) })
}

func (a *app) dislikeCurrent(tr *qqmusic.Track) {
	if !a.nowPlaying || a.pl.Current() != tr || !a.canDislike(tr) || a.dislikeState(tr) != 0 {
		return
	}
	d := a.dislikes
	if d == nil || d.profile != a.profile {
		d = &songDislikes{a.profile, make(map[int64]dislikeStatus)}
		a.dislikes = d
	}
	uid, id, radio := a.profile.UserID, tr.ID, a.pl.Radio()
	prior := d.songs[id]
	d.songs[id] = dislikePending
	write := dislikeSong
	go func() {
		err := write(uid, id)
		a.post(func() {
			if a.profile != d.profile || a.dislikes != d {
				return
			}
			if err == nil {
				d.songs[id] = dislikeConfirmed
				if daily := a.dailyDislikes; daily != nil && daily.profile == d.profile {
					daily.record(id)
					daily.writes++
				}
				if a.pl.Current() == tr && a.pl.Radio() == radio {
					a.pl.NextWithoutWrap()
				}
				return
			}
			if prior != 0 {
				d.songs[id] = prior
			} else {
				delete(d.songs, id)
			}
			switch {
			case errors.Is(err, qqmusic.ErrDislikeUnconfirmed):
				a.toast = "未能确认是否已加入 QQ 音乐黑名单"
			case errors.Is(err, qqmusic.ErrLoginRequired), qqmusic.SessionNeedsLogin(err):
				a.toast = "请重新登录 QQ 音乐后再试"
			default:
				a.toast = "没能加入 QQ 音乐黑名单"
			}
		})
	}()
}
