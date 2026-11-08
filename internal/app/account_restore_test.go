package app

import (
	"errors"
	"image/png"
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
	restoreLoginSession = func() bool { restored <- struct{}{}; return saved }
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
	if err := tt.Click("重新扫码登录"); err != nil {
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

// With NM_FRAMES set, saves failed and pending recovery menus in both appearances.
func TestAccountRestoreFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"failed", "pending"} {
		for _, dark := range []bool{false, true} {
			a := newTestApp()
			a.settings = defaultSettings()
			a.accountRestore = accountRestoreState{failed: true, loading: state == "pending"}
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
