package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// loginState is a QR code login under way.
type loginState struct {
	open     bool
	gen      int // counts logins, to stop the polls of closed ones
	key      string
	code     *ui.Bitmap
	status   int // a QR state, or 0 while the code loads
	scanner  string
	avatar   string
	err      string
	finished bool
	wechat   bool
}

type accountRestoreState struct {
	gen     int
	loading bool
	failed  bool
	boot    bool
}

// beginLoginQR is replaced by UI tests so retry actions never contact QQ.
var beginLoginQR = qqmusic.QRKey
var beginWechatLoginQR = qqmusic.WechatQRKey

// Restoration tests isolate credential reads and the page loads that follow a recovered account.
var restoreLoginSession = qqmusic.RestoreSession
var readLoginAccount = qqmusic.Account
var applyLoginProfile = (*app).signedIn

// boot signs in with the session in the keychain, if any, then loads the
// home page for whoever is listening.
func (a *app) boot() {
	a.homeLoading = true
	a.startAccountRestore(true)
}

// startAccountRestore reads saved credentials at boot; retries use the retained in-memory session.
func (a *app) startAccountRestore(boot bool) {
	if a.accountRestore.loading {
		return
	}
	a.accountRestore.gen++
	a.accountRestore.loading = true
	a.accountRestore.boot = boot
	gen := a.accountRestore.gen
	go func() {
		var prof *qqmusic.Profile
		var err error
		if !boot || restoreLoginSession() {
			prof, err = readLoginAccount()
		}
		a.post(func() {
			if !a.acceptAccountRestore(gen, prof, err) {
				return
			}
			if boot || err == nil {
				applyLoginProfile(a, a.profile)
				a.accountRestore.failed = err != nil
			}
		})
	}()
}

// acceptAccountRestore applies only the active request; failure keeps the session available to retry.
func (a *app) acceptAccountRestore(gen int, prof *qqmusic.Profile, err error) bool {
	if gen != a.accountRestore.gen || !a.accountRestore.loading {
		return false
	}
	a.accountRestore.loading = false
	a.accountRestore.boot = false
	a.accountRestore.failed = err != nil
	if err == nil {
		a.profile = prof
	}
	return true
}

// signedIn shows what is personal to prof, or to nobody.
func (a *app) signedIn(prof *qqmusic.Profile) {
	a.accountRestore = accountRestoreState{gen: a.accountRestore.gen + 1}
	a.profile = prof
	a.resetSongFans()
	// Playback picks up where it was left, once the session that lets
	// the listener play their songs is back.
	a.restoreSession()
	a.syncSongFans()
	a.mine, a.saved, a.recPlaylists, a.liked = nil, nil, nil, nil
	a.clearSongShelves()
	a.likedPrograms, a.askedPrograms = nil, nil
	a.likedAlbums = nil
	a.likedScroll = likedScrollState{}
	// What the listener may play, and follows, changed: load the pages
	// again.
	a.playlists = map[int64]*qqmusic.Playlist{}
	a.plErr = map[int64]string{}
	a.artists, a.albums, a.users, a.podcasts = map[int64]*artistData{}, map[int64]*albumData{}, map[int64]*userData{}, map[int64]*podcastData{}
	a.podcastTab = &podcastTab{}
	a.purchased = [2]*purchasedData{}
	a.purchasedScroll = [2]listScrollState{}
	a.searches = map[route]*searchData{}
	a.load(a.route)
	a.loadHome()
	if prof == nil {
		return
	}
	a.loadLiked(prof.UserID)
	a.loadMine(prof.UserID)
}

func (a *app) openLogin() {
	a.openLoginChannel(false)
}

func (a *app) openLoginChannel(wechat bool) {
	if a.accountRestore.loading && a.accountRestore.boot {
		// Choosing a fresh login must still finish startup if the QR is later dismissed.
		applyLoginProfile(a, nil)
	}
	a.accountRestore.gen++
	a.accountRestore.loading = false
	a.accountRestore.boot = false
	qqmusic.CancelQR(a.login.key)
	a.login = loginState{open: true, gen: a.login.gen + 1, wechat: wechat}
	gen := a.login.gen
	begin := beginLoginQR
	if wechat {
		begin = beginWechatLoginQR
	}
	go func() {
		key, err := begin()
		var code *ui.Bitmap
		if err == nil {
			var png []byte
			png, err = qqmusic.QRImage(key)
			if err == nil {
				code, err = ui.DecodeBitmap(png)
				if err != nil {
					err = &qqmusic.LoginError{Step: "qr-image", Err: err}
				}
			}
		}
		a.post(func() {
			if a.acceptLoginQR(gen, key, code, err) {
				a.pollLogin(gen, key)
			}
		})
	}()
}

// acceptLoginQR applies a worker result on the main thread before polling starts.
func (a *app) acceptLoginQR(gen int, key string, code *ui.Bitmap, err error) bool {
	if gen != a.login.gen || !a.login.open {
		qqmusic.CancelQR(key)
		return false
	}
	if err != nil {
		a.login.err = qqmusic.LoginErrorMessage(err)
		return false
	}
	a.login.key, a.login.code, a.login.status, a.login.err = key, code, qqmusic.QRWaiting, ""
	return true
}

// pollLogin asks every two seconds whether the code was scanned, until the
// login ends or the dialog closes.
func (a *app) pollLogin(gen int, key string) {
	go func() {
		for {
			time.Sleep(2 * time.Second)
			code, msg, name, avatar, err := qqmusic.QRCheck(key)
			var prof *qqmusic.Profile
			if code == qqmusic.QRConfirmed {
				prof, err = qqmusic.Account()
				if err == nil {
					err = qqmusic.SaveSession()
				}
			}
			done := make(chan bool, 1)
			a.post(func() {
				l := &a.login
				if gen != l.gen || !l.open {
					done <- true
					return
				}
				switch {
				case err != nil:
					l.err = qqmusic.LoginErrorMessage(err)
				case code == qqmusic.QRConfirmed:
					if prof == nil {
						l.err = "未能获取 QQ 音乐账号，请刷新二维码"
						break
					}
					l.status, l.finished = code, true
					a.signedIn(prof)
					a.closeLoginSoon(gen)
				case code == qqmusic.QRExpired || code == qqmusic.QRWaiting || code == qqmusic.QRScanned:
					l.status = code
					if name != "" {
						l.scanner, l.avatar = name, avatar
					}
				default:
					l.err = msg
					if l.err == "" {
						l.err = "登录失败，请稍后再试"
					}
				}
				done <- l.status == qqmusic.QRExpired || l.finished || l.err != ""
			})
			if <-done {
				return
			}
		}
	}()
}

// closeLoginSoon closes the dialog a moment after it showed the welcome.
func (a *app) closeLoginSoon(gen int) {
	go func() {
		time.Sleep(1200 * time.Millisecond)
		a.post(func() {
			if gen == a.login.gen {
				a.login.open = false
			}
		})
	}()
}

func (a *app) signOut() {
	qqmusic.CancelQR(a.login.key)
	a.login.gen++
	qqmusic.Logout()
	a.signedIn(nil)
}

// loginDialog shows the selected QQ or WeChat QR code.
func (a *app) loginDialog(c *ui.Context) {
	p := a.pal
	l := &a.login
	ui.DialogBase(c.Key("login"), &l.open, func(backdrop, panel ui.Element) {
		backdrop.Background(radixInk.Alpha(0.35))
		panel.Width(340).Padding(28, 28, 26, 28).Radius(22).Gap(6).AlignItems(ui.Center).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, radixInk.Alpha(0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		channel := "QQ"
		if l.wechat {
			channel = "微信"
		}
		title, sub := "扫码登录", "打开"+channel+"，扫一扫登录"
		switch {
		case l.finished:
			title, sub = "登录成功", "欢迎回来，"+a.profileName()
		case l.err != "":
			title, sub = "登录失败", l.err
		case l.status == qqmusic.QRScanned:
			title, sub = "扫描成功", "请在手机上确认登录"
		case l.status == qqmusic.QRExpired:
			sub = "二维码已失效"
		}
		ui.Text(c, title).FontSize(20).FontWeight(700)
		if !l.finished {
			kind := 0
			if l.wechat {
				kind = 1
			}
			if segmented(c.Key("login-channel"), &kind, "QQ", "微信") {
				a.openLoginChannel(kind == 1)
			}
		}
		ui.Text(c, sub).Width(284).MaxLines(3).FontSize(13).TextColor(p.muted).TextAlign(ui.Center).Margin(0, 0, 14, 0)

		ui.Box(c).Size(212, 212).Radius(16).Background(radixPaper).Border(1, p.hairline).Children(func() {
			if l.code != nil {
				code := l.code
				opacity := float32(1)
				if l.status == qqmusic.QRScanned || l.status == qqmusic.QRExpired || l.finished || l.err != "" {
					opacity = 0.12
				}
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Opacity(opacity).
					Children(func() { ui.Image(c, code).Fill().Fit(ui.Contain) })
			} else if l.err == "" {
				// On white whatever the appearance, as the code is.
				s := shimmer{base: radixBlack(3), sheen: radixBlack(1)}.in(c)
				qrSkeleton(c, s, 14).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			}
			ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Gap(10).Children(func() {
				switch {
				case l.status == qqmusic.QRExpired || l.err != "":
					if pillButton(c.Key("refresh"), iconRepeat, "刷新二维码", p.accent, p.onAccent).Clicked() {
						a.openLoginChannel(l.wechat)
					}
				case l.finished || l.status == qqmusic.QRScanned:
					if bmp := bitmap(a.art.get(qqmusic.Sized(l.avatar, 160), artCover)); bmp != nil {
						pic := ui.Box(c).Size(72, 72).Radius(36).Shadow(0, 6, 16, -4, radixInk.Alpha(0.3))
						in := artIn(pic, bmp)
						pic.Children(func() { artImage(c, bmp, 36, in) })
					}
					if l.scanner != "" {
						ui.Text(c, l.scanner).FontSize(14).FontWeight(600).TextColor(radixSage(false, 12))
					}
				}
			})
		})
	})
	if !l.open {
		qqmusic.CancelQR(l.key)
	}
}

func (a *app) profileName() string {
	if a.profile == nil {
		return ""
	}
	return a.profile.Nickname
}

// accountRow sits at the foot of the sidebar: who is signed in, with a
// dropdown to open their page or sign out, or a way to sign in.
func (a *app) accountRow(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c.Key("account")).Height(40).PaddingX(8).Gap(10).Radius(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || a.accountOpen {
		b.Background(a.sidebarSurface(c, p.hover, 0.035))
	}
	if a.profile == nil {
		status := "未登录"
		if a.accountRestore.loading {
			status = "正在恢复登录"
		} else if a.accountRestore.failed {
			status = "登录恢复失败"
		}
		b.Label("账户").Expanded(a.accountOpen).Children(func() {
			ui.Box(c).Size(26, 26).Radius(13).Background(p.press).Center().Children(func() {
				ui.Icon(c, iconUser).Size(14, 14).TextColor(p.muted)
			})
			ui.Text(c, status).FontSize(13).FontWeight(500).TextColor(p.muted).Grow(1)
			ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
		})
		if b.Clicked() {
			a.accountOpen = !a.accountOpen
		}
		a.accountMenu(c, b)
		return
	}
	prof := a.profile
	b.Label(prof.Nickname).Expanded(a.accountOpen).Children(func() {
		a.avatar(c, prof.AvatarURL, 26, p.press)
		ui.Text(c, prof.Nickname).FontSize(13).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
	})
	if b.Clicked() {
		a.accountOpen = !a.accountOpen
	}
	a.accountMenu(c, b)
}

// accountMenu drops up from the account row, as wide as it, in the colors
// of the volume popover: the listener's own page and signing out.
func (a *app) accountMenu(c *ui.Context, row ui.Element) {
	pop := popoverColors(a.pal.dark)
	ui.PopoverBase(c.Key("account-menu"), row, &a.accountOpen, func(panel ui.Element) {
		panel.AttachTo(row, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).Padding(5).Gap(1).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, radixInk.Alpha(0.2)).
			Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label("账户").TransformOrigin(0.5, 1).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.96, Y: 4}, Exit: &ui.Motion{Scale: 0.98}})
		if w := row.Bounds().W; w > 0 {
			panel.Width(w)
		}
		if a.profile == nil {
			r := a.accountRestore
			if r.failed || r.loading {
				retry := a.menuItem(c, iconRepeat, "重试恢复登录", "").Disabled(r.loading)
				if !r.loading {
					retry.AutoFocus()
				}
				if retry.Clicked() {
					a.accountOpen = false
					a.startAccountRestore(false)
				}
			}
			label := "登录 QQ 音乐"
			if r.failed {
				label = "重新扫码登录"
			}
			scan := a.menuItem(c, iconUser, label, "")
			if !r.failed || r.loading {
				scan.AutoFocus()
			}
			if scan.Clicked() {
				a.accountOpen = false
				a.openLogin()
			}
		} else {
			if a.menuItem(c, iconUser, "我的主页", "").AutoFocus().Clicked() {
				a.accountOpen = false
				a.open(route{page: pageUser, id: a.profile.UserID})
			}
		}
		if a.profile != nil {
			ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
			if a.menuItem(c, iconSignOut, "退出登录", "").Clicked() {
				a.accountOpen = false
				a.signOut()
			}
		}
	})
}

// menuItem is an item of a dropdown: an icon, its label, and the shortcut
// that does the same, if any.
func (a *app) menuItem(c *ui.Context, icon *ui.SVG, label, shortcut string) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c.Key(label)).Role(ui.RoleMenuItem).Label(label).Height(30).PaddingX(8).Gap(9).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.press)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(p.muted)
		ui.Text(c, label).FontSize(13).Grow(1)
		if shortcut != "" {
			ui.Text(c, shortcut).FontSize(12).TextColor(p.faint)
		}
	})
	return b
}

// myPlaylists lists the listener's playlists under a header that opens
// and closes them: those they made, if mine, with a button to make one
// and a menu to rename them.
func (a *app) myPlaylists(c *ui.Context, title string, lists []qqmusic.Playlist, mine bool, collapsed *bool) {
	if len(lists) == 0 {
		return
	}
	p := a.pal
	ui.Row(c.Key(title)).AlignItems(ui.Center).Margin(18, 4, 6, 6).Children(func() {
		b := ui.ButtonBase(c.Key("toggle")).Grow(1).Height(26).PaddingX(4).Gap(6).Radius(6).
			AlignItems(ui.Center).Justify(ui.Start).Role(ui.RoleDisclosure).Label(title)
		if b.Clicked() {
			*collapsed = !*collapsed
		}
		b.Expanded(!*collapsed)
		if b.Hovered() {
			b.Background(a.sidebarSurface(c, p.hover, 0.035))
		}
		b.Children(func() {
			ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(p.faint).Grow(1)
			arrow := iconDown
			if *collapsed {
				arrow = iconForward
			}
			ui.Icon(c, arrow).Size(12, 12).TextColor(p.faint)
		})
		if mine && iconButton(c.Key("new"), iconPlus, 13, p.faint, "新建歌单").Size(26, 26).Radius(6).Clicked() {
			a.newPlaylist(nil)
		}
	})
	if *collapsed {
		return
	}
	for i := range lists {
		pl := &lists[i]
		selected := a.route.page == pagePlaylist && a.route.id == pl.ID && !a.nowPlaying
		b := ui.ButtonBase(c.Key(pl.ID)).Height(34).PaddingX(7).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
		a.styleSidebarItem(c, b, selected)
		b.Children(func() {
			a.cover(c, pl.Cover(), 80, 22, 5)
			ui.Text(c, pl.Name).FontSize(13).TextColor(p.text).SingleLine().Shrink(1).MinWidth(0)
		})
		if mine && pl.SpecialType != 5 {
			b.ContextMenu(func(m *ui.Menu) {
				if m.Item("重命名…").Chosen() {
					a.startRename(pl)
				}
				m.Separator()
				if m.Item("删除歌单…").Chosen() {
					a.confirmDelete(pl)
				}
			})
		} else if !mine {
			b.ContextMenu(func(m *ui.Menu) {
				if m.Item("取消收藏…").Chosen() {
					a.confirmUnfavorite(pl)
				}
			})
		}
		if b.Clicked() {
			a.open(route{page: pagePlaylist, id: pl.ID})
		}
	}
}
