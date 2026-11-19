package app

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

type accountRestoreReply struct {
	profile *qqmusic.Profile
	err     error
}

func fakeAccountRestore(t *testing.T, saved bool, replies ...accountRestoreReply) (chan struct{}, chan accountRestoreReply) {
	t.Helper()
	oldRestore, oldRead := restoreLoginSession, readLoginAccount
	restored, asked := make(chan struct{}, 8), make(chan accountRestoreReply, 8)
	responses := make(chan accountRestoreReply, len(replies))
	for _, r := range replies {
		responses <- r
	}
	restoreLoginSession = func() (bool, error) { restored <- struct{}{}; return saved, nil }
	readLoginAccount = func() (*qqmusic.Profile, error) {
		var r accountRestoreReply
		select {
		case r = <-responses:
		default:
			r.err = errors.New("unexpected account read")
		}
		asked <- r
		return r.profile, r.err
	}
	t.Cleanup(func() { restoreLoginSession, readLoginAccount = oldRestore, oldRead })
	return restored, asked
}

// A failed startup restore must retry the retained account without another scan and remain retryable until it succeeds.
func TestAccountRestoreRetry(t *testing.T) {
	profile := &qqmusic.Profile{UserID: 42, Nickname: "Listener"}
	restored, asked := fakeAccountRestore(t, true,
		accountRestoreReply{err: errors.New("offline")},
		accountRestoreReply{err: errors.New("still offline")},
		accountRestoreReply{profile: profile},
	)
	a := newTestApp()
	a.settings = defaultSettings()
	a.boot()
	wait(t, restored, "saved credentials")
	first := wait(t, asked, "the startup account read")
	// post drops results without a native window; apply the same worker result on the test's main thread.
	if !a.acceptAccountRestore(a.accountRestore.gen, first.profile, first.err) || !a.accountRestore.failed || a.profile != nil {
		t.Fatal("startup failure was treated as an ordinary signed-out account")
	}
	a.homeLoading = false
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	for attempt := 0; attempt < 2; attempt++ {
		if err := tt.Click("账户"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if attempt == 0 {
			if err := tt.Click("重试恢复登录"); err != nil {
				t.Fatal(err)
			}
		} else {
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
		}
		if !a.accountRestore.loading || a.accountOpen || a.login.open {
			t.Fatal("retry did not start account recovery or opened a QR dialog")
		}
		gen := a.accountRestore.gen
		r := wait(t, asked, "the retried account read")
		if len(restored) != 0 {
			t.Fatal("retry reloaded saved credentials instead of using the retained session")
		}
		tt.Frame()
		if err := tt.Click("账户"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		_ = tt.Click("重试恢复登录")
		if a.accountRestore.gen != gen || len(asked) != 0 {
			t.Fatal("a second click started a duplicate recovery")
		}
		a.accountOpen = false
		if !a.acceptAccountRestore(gen, r.profile, r.err) {
			t.Fatal("active recovery result was discarded")
		}
		if attempt == 0 && (!a.accountRestore.failed || a.profile != nil) {
			t.Fatal("failed retry lost the recovery state")
		}
		tt.Frame()
	}
	if a.profile != profile || a.accountRestore.failed || a.accountRestore.loading || a.login.open {
		t.Fatal("successful recovery did not restore the account without scanning")
	}
}

// Switching to QR login must discard an older recovery result instead of signing into the previous account.
func TestAccountRestoreDoesNotOverrideQRLogin(t *testing.T) {
	_, asked := fakeAccountRestore(t, true, accountRestoreReply{profile: &qqmusic.Profile{UserID: 7, Nickname: "Old account"}})
	qrAsked := make(chan struct{}, 1)
	oldQR := beginLoginQR
	beginLoginQR = func() (string, error) { qrAsked <- struct{}{}; return "", errors.New("QR offline") }
	t.Cleanup(func() { beginLoginQR = oldQR })
	a := newTestApp()
	a.settings = defaultSettings()
	a.accountRestore.failed = true
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Click("账户")
	tt.Frame()
	if err := tt.Click("重试恢复登录"); err != nil {
		t.Fatal(err)
	}
	gen := a.accountRestore.gen
	r := wait(t, asked, "the pending recovery")
	tt.Frame()
	tt.Click("账户")
	tt.Frame()
	if err := tt.Click("登录 QQ 音乐"); err != nil {
		t.Fatal(err)
	}
	wait(t, qrAsked, "a replacement QR login")
	if a.acceptAccountRestore(gen, r.profile, r.err) || a.profile != nil || a.accountRestore.loading || !a.login.open {
		t.Fatal("old recovery replaced the new QR login")
	}
}

// Replacing startup recovery with a QR login must still initialize the pages when the QR fails or is dismissed.
func TestAccountRestoreBootReplacedByQR(t *testing.T) {
	restored, asked := fakeAccountRestore(t, true, accountRestoreReply{profile: &qqmusic.Profile{UserID: 7, Nickname: "Old account"}})
	qrAsked := make(chan struct{}, 1)
	oldQR, oldApply := beginLoginQR, applyLoginProfile
	beginLoginQR = func() (string, error) { qrAsked <- struct{}{}; return "", errors.New("QR offline") }
	initialized := 0
	applyLoginProfile = func(a *app, p *qqmusic.Profile) {
		if p != nil {
			t.Error("startup replacement used the old account")
		}
		initialized++
		a.homeLoading = false // the isolated page loader has completed
	}
	t.Cleanup(func() { beginLoginQR, applyLoginProfile = oldQR, oldApply })
	a := newTestApp()
	a.settings = defaultSettings()
	a.boot()
	wait(t, restored, "startup credentials")
	r := wait(t, asked, "startup recovery")
	gen := a.accountRestore.gen
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Click("账户")
	tt.Frame()
	if err := tt.Click("登录 QQ 音乐"); err != nil {
		t.Fatal(err)
	}
	wait(t, qrAsked, "a replacement startup QR")
	if initialized != 1 || a.homeLoading || a.accountRestore.loading || !a.login.open {
		t.Fatal("replacing boot recovery left the initial pages waiting for a discarded request")
	}
	a.acceptLoginQR(a.login.gen, "", nil, errors.New("QR offline"))
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.acceptAccountRestore(gen, r.profile, r.err) || a.profile != nil || a.login.open || initialized != 1 {
		t.Fatal("dismissed QR accepted an old recovery or restarted initialization")
	}
}

// A saved session with no usable account must return to normal sign-in rather than offering endless recovery retries.
func TestAccountRestoreWithoutAccount(t *testing.T) {
	_, asked := fakeAccountRestore(t, true, accountRestoreReply{})
	a := newTestApp()
	a.settings = defaultSettings()
	a.accountRestore.failed = true
	a.startAccountRestore(false)
	r := wait(t, asked, "an unavailable account")
	if !a.acceptAccountRestore(a.accountRestore.gen, r.profile, r.err) || a.accountRestore.failed || a.accountRestore.loading || a.profile != nil {
		t.Fatal("missing account stayed in a failed restore state")
	}
	qrAsked := make(chan struct{}, 1)
	oldQR := beginLoginQR
	beginLoginQR = func() (string, error) { qrAsked <- struct{}{}; return "", errors.New("QR offline") }
	t.Cleanup(func() { beginLoginQR = oldQR })
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Click("账户")
	tt.Frame()
	if err := tt.Click("登录 QQ 音乐"); err != nil {
		t.Fatal(err)
	}
	wait(t, qrAsked, "normal sign-in")
	if !a.login.open || a.accountRestore.loading {
		t.Fatal("normal sign-in was blocked after recovery found no account")
	}
}

func TestAccountRestoreStorageFailureRetriesRead(t *testing.T) {
	oldRestore, oldRead := restoreLoginSession, readLoginAccount
	readErr := &qqmusic.SessionError{Stage: "read", Err: errors.New("private-storage-error")}
	restored, asked := make(chan error, 2), make(chan struct{}, 1)
	calls := 0
	restoreLoginSession = func() (bool, error) {
		calls++
		if calls == 1 {
			restored <- readErr
			return false, readErr
		}
		restored <- nil
		return true, nil
	}
	readLoginAccount = func() (*qqmusic.Profile, error) {
		asked <- struct{}{}
		return nil, nil
	}
	t.Cleanup(func() { restoreLoginSession, readLoginAccount = oldRestore, oldRead })
	a := newTestApp()
	a.settings = defaultSettings()
	a.boot()
	err := wait(t, restored, "failed credential read")
	// There is no native window in this test; apply the worker result explicitly.
	if !a.acceptAccountRestore(a.accountRestore.gen, nil, err) || !a.accountRestore.reload || len(asked) != 0 {
		t.Fatal("failed credential read was hidden or attempted an anonymous account lookup")
	}
	a.homeLoading = false
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Click("账户")
	tt.Frame()
	if !tt.HasText(qqmusic.SessionErrorMessage(readErr)) {
		t.Fatal("credential read failure reason was not shown in the account menu")
	}
	if err := tt.Click("重试恢复登录"); err != nil {
		t.Fatal(err)
	}
	if err := wait(t, restored, "retried credential read"); err != nil {
		t.Fatal(err)
	}
	wait(t, asked, "account lookup after the credential read recovered")
}

func TestAccountRestoreUnrecoverableDataOffersScan(t *testing.T) {
	for _, stage := range []string{"decode", "unusable"} {
		t.Run(stage, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.accountRestore = accountRestoreState{gen: 1, loading: true}
			err := &qqmusic.SessionError{Stage: stage}
			if !a.acceptAccountRestore(1, nil, err) {
				t.Fatal("recovery result was discarded")
			}
			tt := ui.NewTester(a.view, 1000, 700)
			tt.Frame()
			tt.Click("账户")
			tt.Frame()
			if !tt.HasText(qqmusic.SessionErrorMessage(err)) || !tt.HasText("重新扫码登录") || tt.HasText("重试恢复登录") {
				t.Fatal("unusable saved data did not show its reason and a fresh scan action")
			}
		})
	}
}

func TestAccountRecoveryClearsLoginPlaybackError(t *testing.T) {
	oldHomeF, oldHomeC, oldHomeD, oldHomeR := homeFeatured, homeCharts, homeDaily, homeRecommended
	oldShelves, oldPlaylists, oldLiked := readSongShelves, userPlaylists, readLikedIDs
	homeFeatured = func(int) ([]qqmusic.Playlist, error) { return nil, nil }
	homeCharts = func() ([]qqmusic.Playlist, error) { return nil, nil }
	homeDaily = func() ([]qqmusic.Track, error) { return nil, nil }
	homeRecommended = func() ([]qqmusic.Playlist, error) { return nil, nil }
	readSongShelves = func() ([]qqmusic.SongShelf, error) { return nil, nil }
	userPlaylists = func(int64) ([]qqmusic.Playlist, []qqmusic.Playlist, error) { return nil, nil, nil }
	readLikedIDs = func(int64) ([]int64, error) { return nil, nil }
	t.Cleanup(func() {
		homeFeatured, homeCharts, homeDaily, homeRecommended = oldHomeF, oldHomeC, oldHomeD, oldHomeR
		readSongShelves, userPlaylists, readLikedIDs = oldShelves, oldPlaylists, oldLiked
	})

	a := newTestApp()
	a.settings = defaultSettings()
	a.restored = true
	a.pl.Cue([]qqmusic.Track{{ID: 1, MID: "m1", Name: "Song"}})
	a.pl.SetErr(qqmusic.ErrLoginRequired.Error())

	// Signing out (prof == nil) must keep the login prompt error.
	a.signedIn(nil)
	if a.pl.Err() != qqmusic.ErrLoginRequired.Error() {
		t.Fatalf("signedIn(nil) cleared login error: %q", a.pl.Err())
	}

	// Recovering the account clears the login-required playback error.
	a.signedIn(&qqmusic.Profile{UserID: 42, Nickname: "Listener"})
	if a.pl.Err() != "" {
		t.Fatalf("unexpected playback error after sign-in: %q", a.pl.Err())
	}

	// Non-login errors (e.g. VIP-only) must not be cleared on sign-in.
	const vipErr = "这首歌需要 VIP 才能播放"
	a.pl.SetErr(vipErr)
	a.signedIn(&qqmusic.Profile{UserID: 42, Nickname: "Listener"})
	if a.pl.Err() != vipErr {
		t.Fatalf("non-login playback error was cleared on sign-in: %q", a.pl.Err())
	}
}

// With NM_FRAMES set, saves failed and pending recovery menus in both appearances.
func TestAccountRestoreFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"failed", "storage", "rescan", "pending"} {
		for _, dark := range []bool{false, true} {
			a := newTestApp()
			a.settings = defaultSettings()
			a.accountRestore = accountRestoreState{failed: true, loading: state == "pending"}
			switch state {
			case "failed":
				a.setAccountRestoreError(errors.New("offline"))
			case "storage":
				a.setAccountRestoreError(&qqmusic.SessionError{Stage: "read"})
			case "rescan":
				a.setAccountRestoreError(&qqmusic.SessionError{Stage: "unusable"})
			}
			a.accountOpen = true
			tt := ui.NewTester(a.view, 1000, 700)
			tt.SetDark(dark)
			tt.Advance(time.Second)
			tt.Frame()
			name := "account-restore-" + state
			if dark {
				name += "-dark"
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// An authorization error after scanning must offer retry instead of leaving the scanned overlay stuck.
func TestLoginErrorAfterScanCanRefresh(t *testing.T) {
	asked := make(chan struct{}, 1)
	old := beginLoginQR
	beginLoginQR = func() (string, error) {
		asked <- struct{}{}
		return "", errors.New("offline")
	}
	t.Cleanup(func() { beginLoginQR = old })

	a := newTestApp()
	a.settings = defaultSettings()
	a.login = loginState{open: true, gen: 7, key: "old-key", status: qqmusic.QRScanned, scanner: "listener", err: "authorization failed"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	if err := tt.Click("刷新二维码"); err != nil {
		t.Fatal(err)
	}
	if !a.login.open || a.login.gen != 8 || a.login.status != 0 || a.login.key != "" || a.login.err != "" || a.login.scanner != "" {
		t.Fatalf("retry retained the failed login: %+v", a.login)
	}
	wait(t, asked, "a new QQ login QR")
	tt.Frame()
}

// A failed QR fetch must permit another refresh, and its successful response must replace the old state.
func TestLoginQRRefreshAfterFetchFailure(t *testing.T) {
	asked := make(chan struct{}, 2)
	old := beginLoginQR
	beginLoginQR = func() (string, error) { asked <- struct{}{}; return "", errors.New("offline") }
	t.Cleanup(func() { beginLoginQR = old })
	a := newTestApp()
	a.settings = defaultSettings()
	a.login = loginState{open: true, gen: 1, err: "previous failure"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	if err := tt.Click("刷新二维码"); err != nil {
		t.Fatal(err)
	}
	wait(t, asked, "the first QR refresh")
	first := a.login.gen
	failure := &qqmusic.LoginError{Step: "qr-show", Err: &url.Error{Op: "Get", URL: "https://xui.ptlogin2.qq.com/ssl/ptqrshow?private=fake", Err: io.EOF}}
	if a.acceptLoginQR(first, "", nil, failure) || a.login.err == "" || a.login.key != "" || a.login.code != nil {
		t.Fatal("failed fetch left a ready QR")
	}
	tt.Frame()
	if err := tt.Click("刷新二维码"); err != nil {
		t.Fatal(err)
	}
	wait(t, asked, "the second QR refresh")
	second := a.login.gen
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 37, 37)))
	if a.acceptLoginQR(first, "stale", bitmap, nil) {
		t.Fatal("old QR response replaced the new refresh")
	}
	if !a.acceptLoginQR(second, "fresh", bitmap, nil) {
		t.Fatal("fresh QR was discarded")
	}
	if a.login.key != "fresh" || a.login.code != bitmap || a.login.status != qqmusic.QRWaiting || a.login.err != "" {
		t.Fatal("fresh QR did not replace the failed fetch")
	}
	tt.Frame()
}

// Switching login channels replaces the QR generation, and refresh keeps the selected channel.
func TestLoginSwitchesToWechatAndRefreshes(t *testing.T) {
	asked := make(chan struct{}, 2)
	old := beginWechatLoginQR
	beginWechatLoginQR = func() (string, error) { asked <- struct{}{}; return "", errors.New("offline") }
	t.Cleanup(func() { beginWechatLoginQR = old })
	a := newTestApp()
	a.settings = defaultSettings()
	a.login = loginState{open: true, gen: 4, key: "old-qq", status: qqmusic.QRWaiting}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	if err := tt.Click("微信"); err != nil {
		t.Fatal(err)
	}
	wait(t, asked, "a WeChat QR")
	if !a.login.wechat || a.login.gen != 5 || a.login.key != "" {
		t.Fatal("switch retained QQ login")
	}
	if a.acceptLoginQR(4, "stale-qq", nil, nil) {
		t.Fatal("old QQ result replaced WeChat login")
	}
	a.acceptLoginQR(5, "", nil, errors.New("offline"))
	tt.Frame()
	if err := tt.Click("刷新二维码"); err != nil {
		t.Fatal(err)
	}
	wait(t, asked, "a refreshed WeChat QR")
	if !a.login.wechat || a.login.gen != 6 {
		t.Fatal("refresh changed the login channel")
	}
	tt.Frame()
}

// With NM_FRAMES set, saves the WeChat login layout using an inert QR-like image.
func TestWechatLoginFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	qr := image.NewRGBA(image.Rect(0, 0, 45, 45))
	for y := 0; y < 45; y++ {
		for x := 0; x < 45; x++ {
			c := color.White
			if x > 3 && x < 41 && y > 3 && y < 41 && ((x*11+y*7+x*y)%5 < 2) {
				c = color.Black
			}
			qr.Set(x, y, c)
		}
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.login = loginState{open: true, wechat: true, status: qqmusic.QRWaiting, code: ui.NewBitmap(qr)}
		tt := ui.NewTester(a.view, 1000, 700)
		tt.SetDark(dark)
		tt.Advance(time.Second)
		tt.Frame()
		name := "login-wechat.png"
		if dark {
			name = "login-wechat-dark.png"
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
