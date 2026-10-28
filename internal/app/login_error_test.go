package app

import (
	"errors"
	"image"
	"io"
	"net/url"
	"testing"

	"github.com/egoist/mygo/ui"

	"qqmusic-mygo/internal/qqmusic"
)

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
