package qqmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

type authRoundTripper func(*http.Request) (*http.Response, error)

func (fn authRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func fakeAuthServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	base, _ := url.Parse(server.URL)
	old := authHTTP
	authHTTP = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		clone.URL = &u
		clone.Host = r.URL.Host
		clone.URL.Scheme = base.Scheme
		clone.URL.Host = base.Host
		return http.DefaultTransport.RoundTrip(clone)
	})}
	authState.Lock()
	prior := authState.session
	priorPending := authState.pending
	priorQR := authState.qr
	authState.session = Session{}
	authState.pending = false
	authState.version++
	authState.qr = nil
	authState.Unlock()
	t.Cleanup(func() {
		authHTTP = old
		authState.Lock()
		authState.session = prior
		authState.pending = priorPending
		authState.version++
		authState.qr = priorQR
		authState.Unlock()
	})
}

// A failed fetch must not poison later QR creation or return the previous QR's bytes.
func TestQRRefreshAfterFetchFailure(t *testing.T) {
	var pngBytes bytes.Buffer
	_ = png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 37, 37)))
	requests := 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "qrsig", Value: fmt.Sprintf("fresh-sig-%d", requests), Domain: "ptlogin2.qq.com", Path: "/"})
		_, _ = w.Write(pngBytes.Bytes())
	})
	if _, err := QRKey(); err == nil {
		t.Fatal("failed request looked successful")
	}
	var previous string
	for i := 0; i < 2; i++ {
		key, err := QRKey()
		if err != nil {
			t.Fatal(err)
		}
		if key == previous {
			t.Fatal("refresh reused the old QR key")
		}
		image, err := QRImage(key)
		if err != nil || !bytes.Equal(image, pngBytes.Bytes()) {
			t.Fatal("fresh QR image was not retained")
		}
		previous = key
	}
}

// QQ's scan must decode escaped callback parameters and retain graph cookies despite parent-domain deletion cookies.
func TestQQQRLogin(t *testing.T) {
	const sig = "fake-qrsig-value_01"
	const oauthCode = "ABCDEF0123456789ABCDEF0123456789"
	var imageBytes bytes.Buffer
	_ = png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 37, 37)))
	for _, order := range []string{"graph first", "parent deletion first"} {
		t.Run(order, func(t *testing.T) {
			polls, checks, exchanges := 0, 0, 0
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Host + r.URL.Path {
				case "xui.ptlogin2.qq.com/ssl/ptqrshow":
					if r.URL.Query().Get("appid") != "716027609" || r.URL.Query().Get("pt_3rd_aid") != "100497308" {
						t.Error("incorrect QQ application IDs")
					}
					http.SetCookie(w, &http.Cookie{Name: "qrsig", Value: sig, Domain: "ptlogin2.qq.com", Path: "/"})
					w.Write(imageBytes.Bytes())
				case "xui.ptlogin2.qq.com/ssl/ptqrlogin":
					if r.Header.Get("Cookie") != "qrsig="+sig || r.URL.Query().Get("ptqrtoken") != strconv.FormatUint(uint64(hash33(sig, 0)), 10) {
						t.Error("poll lost QR authentication")
					}
					polls++
					switch polls {
					case 1:
						io.WriteString(w, `ptuiCB('66','0','','0','waiting','')`)
					case 2:
						io.WriteString(w, `ptuiCB('67','0','','0','scanned','')`)
					default:
						io.WriteString(w, `ptuiCB('0','0','https://ssl.ptlogin2.graph.qq.com/check_sig?ptsigx=fake\x26daid=383\u0026pt_3rd_aid=100497308','0','ok','nick')`)
					}
				case "ssl.ptlogin2.graph.qq.com/check_sig":
					checks++
					q := r.URL.Query()
					if len(q) != 3 || q.Get("ptsigx") != "fake" || q.Get("daid") != "383" || q.Get("pt_3rd_aid") != "100497308" {
						t.Error("escaped callback separators did not reach check_sig as separate query parameters")
					}
					for _, c := range []http.Cookie{{Name: "p_uin", Value: "o0010001"}, {Name: "p_skey", Value: "fake-pskey"}, {Name: "pt_oauth_token", Value: "fake-oauth"}} {
						c.Domain, c.Path = "graph.qq.com", "/"
						deleted := http.Cookie{Name: c.Name, Value: "", Domain: "qq.com", Path: "/", Expires: time.Unix(0, 0).UTC()}
						if order == "parent deletion first" {
							http.SetCookie(w, &deleted)
							http.SetCookie(w, &c)
						} else {
							http.SetCookie(w, &c)
							http.SetCookie(w, &deleted)
						}
					}
					w.Header().Set("Location", loginJump)
					w.WriteHeader(302)
				case "graph.qq.com/oauth2.0/login_jump":
					w.WriteHeader(200)
				case "graph.qq.com/oauth2.0/authorize":
					if r.Method != "POST" {
						t.Error("authorization is not POST")
					}
					_ = r.ParseForm()
					if r.Form.Get("g_tk") != strconv.FormatUint(uint64(hash33("fake-pskey", 5381)), 10) {
						t.Error("authorization hash did not use the graph p_skey")
					}
					cookies := r.Cookies()
					if len(cookies) != 3 {
						t.Error("authorization received missing or duplicate credential cookies")
					}
					for name, want := range map[string]string{"p_uin": "o0010001", "p_skey": "fake-pskey", "pt_oauth_token": "fake-oauth"} {
						c, err := r.Cookie(name)
						if err != nil || c.Value != want {
							t.Errorf("authorization lost graph cookie %s", name)
						}
					}
					if len(r.Form.Get("state")) != 32 {
						t.Error("state is missing")
					}
					w.Header().Set("Location", "https://y.qq.com/portal/wx_redirect.html?code="+oauthCode+"&state="+r.Form.Get("state"))
					w.WriteHeader(302)
				case "u.y.qq.com/cgi-bin/musicu.fcg":
					exchanges++
					if r.Header.Get("Cookie") != "" {
						t.Error("global QQ cookies leaked to musicu")
					}
					var body struct {
						Req struct {
							Module string `json:"module"`
							Method string `json:"method"`
							Param  struct {
								Code string `json:"code"`
							} `json:"param"`
						} `json:"req"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.Req.Module != "QQConnectLogin.LoginServer" || body.Req.Method != "QQLogin" || body.Req.Param.Code != oauthCode {
						t.Errorf("incorrect credential request: %+v", body)
					}
					io.WriteString(w, qqWebLoginResponse)
				default:
					t.Errorf("unexpected request %s%s", r.Host, r.URL)
					w.WriteHeader(500)
				}
			})
			key, err := QRKey()
			if err != nil {
				t.Fatal(err)
			}
			b, err := QRImage(key)
			if err != nil || !bytes.Equal(b, imageBytes.Bytes()) {
				t.Fatalf("lost original QR PNG: %v", err)
			}
			for _, want := range []int{QRWaiting, QRScanned, QRConfirmed} {
				code, _, _, _, err := QRCheck(key)
				if err != nil || code != want {
					t.Fatalf("QRCheck = %d, %v; want %d", code, err, want)
				}
			}
			if checks != 1 || exchanges != 1 {
				t.Fatalf("credential requests: check_sig=%d exchanges=%d", checks, exchanges)
			}
			s := sessionSnapshot()
			if s.MusicID != "10001" || s.MusicKey != "fake-musickey" || s.LoginType != 2 || s.RefreshKey != "" || s.RefreshToken != "fake-refresh-token" || !s.canRefresh() {
				t.Fatalf("bad or non-renewable QQ web session: id=%s type=%d", s.MusicID, s.LoginType)
			}
		})
	}
}

// An OAuth callback with the wrong state must never be exchanged for a music credential.
func TestQQLoginRejectsWrongState(t *testing.T) {
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/check_sig":
			for _, name := range []string{"p_uin", "p_skey", "pt_oauth_token"} {
				http.SetCookie(w, &http.Cookie{Name: name, Value: "fake", Domain: "graph.qq.com"})
			}
			w.Header().Set("Location", loginJump)
			w.WriteHeader(302)
		case "/oauth2.0/login_jump":
			w.WriteHeader(200)
		case "/oauth2.0/authorize":
			w.Header().Set("Location", "https://y.qq.com/portal/wx_redirect.html?code=ABCDEF0123456789ABCDEF0123456789&state=wrong")
			w.WriteHeader(302)
		default:
			t.Error("unexpected credential request after state mismatch")
			w.WriteHeader(500)
		}
	})
	if _, err := exchangePtlogin("https://ssl.ptlogin2.graph.qq.com/check_sig"); err == nil {
		t.Fatal("accepted wrong OAuth state")
	}
}

// Redirects outside the QQ authorization flow and executable JSONP are rejected as data.
func TestQQLoginRejectsInvalidProtocol(t *testing.T) {
	for _, body := range []string{`alert(1)`, `ptuiCB('66')`, `ptuiCB('66','0','','0','waiting','');alert(1)`, `ptuiCB('66','0','','0','waiting','',)`} {
		if _, err := parsePtui(body); err == nil {
			t.Errorf("accepted malformed response %q", body)
		}
	}
	args, err := parsePtui(`ptuiCB('67','0','','0','it\'s scanned','\u4f60\x21');`)
	if err != nil || args[4] != "it's scanned" || args[5] != "你!" {
		t.Fatalf("string escapes = %v %v", args, err)
	}
	for _, raw := range []string{"https://evil.test/check_sig", "http://ssl.ptlogin2.graph.qq.com/check_sig", "https://ssl.ptlogin2.graph.qq.com:443/check_sig"} {
		if _, err := trustedLoginURL(raw, "ssl.ptlogin2.graph.qq.com", "/check_sig"); err == nil {
			t.Error(fmt.Sprintf("accepted redirect %s", raw))
		}
	}
}

// An explicitly rejected restored key must be discarded so anonymous requests do not keep using it.
func TestAccountDiscardsRejectedCredential(t *testing.T) {
	for _, code := range []int{1000, 104400, 104401} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": code, "data": map[string]any{}}})
			})
			oldHTTP := httpClient
			httpClient = authHTTP
			t.Cleanup(func() { httpClient = oldHTTP })
			oldDelete := deletePersistedSession
			deleted := false
			deletePersistedSession = func() { deleted = true }
			t.Cleanup(func() { deletePersistedSession = oldDelete })
			setSession(Session{MusicID: "10001", MusicKey: "rejected", LoginType: 2})
			profile, err := Account()
			if !SessionNeedsLogin(err) || profile != nil || sessionSnapshot().MusicKey != "" || !deleted {
				t.Fatalf("rejected account was retained: profile=%v err=%v deleted=%v", profile, err, deleted)
			}
		})
	}
}

// A temporary network failure must preserve the credential for a later retry.
func TestAccountPreservesCredentialOnNetworkFailure(t *testing.T) {
	oldHTTP := httpClient
	oldSession := sessionSnapshot()
	t.Cleanup(func() { httpClient = oldHTTP; setSession(oldSession) })
	httpClient = &http.Client{Transport: authRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("temporary network failure")
	})}
	setSession(Session{MusicID: "10001", MusicKey: "still-valid", LoginType: 2})
	if _, err := Account(); err == nil {
		t.Fatal("network failure was hidden")
	}
	if sessionSnapshot().MusicKey != "still-valid" {
		t.Fatal("network failure discarded the credential")
	}
}

// A failed scan-confirmation request must never print its temporary authorization parameters.
func TestLoginErrorRedactsAuthorizationURL(t *testing.T) {
	cause := &url.Error{Op: "Get", URL: "https://ssl.ptlogin2.graph.qq.com/check_sig?ptsigx=private-signature&uin=private-account", Err: io.EOF}
	err := loginFailure("check-sig", cause)
	for _, message := range []string{err.Error(), LoginErrorMessage(err), LoginErrorMessage(cause)} {
		for _, secret := range []string{"https://", "ptsigx", "private-signature", "private-account"} {
			if strings.Contains(message, secret) {
				t.Errorf("login error disclosed authorization data: %q", message)
			}
		}
	}
	if !errors.Is(err, io.EOF) {
		t.Fatal("lost network failure cause")
	}
	if !strings.Contains(LoginErrorMessage(err), "QQ 授权") {
		t.Fatal("lost failed login step")
	}
}

// Transport failures must be distinguishable from rejected or malformed credential responses.
func TestLoginErrorClassifiesTransportFailure(t *testing.T) {
	cause := &url.Error{Op: "Post", URL: "https://u.y.qq.com/cgi-bin/musicu.fcg?private=fake", Err: errors.New("TLS handshake failure")}
	message := LoginErrorMessage(loginFailure("qq-login", cause))
	if !strings.Contains(message, "N201") || strings.Contains(message, "private=") {
		t.Fatalf("unexpected login diagnostic: %s", message)
	}
}

// The QQ web exchange returns an account number and refresh token, but leaves its display profile empty.
const qqWebLoginResponse = `{"code":0,"req":{"code":0,"data":{"musicid":10001,"str_musicid":"10001","musickey":"fake-musickey","loginType":2,"openid":"fake-openid","unionid":"fake-unionid","encryptUin":"fake-encrypt-uin","refresh_key":"","refresh_token":"fake-refresh-token","nick":"","logo":""}}}`

// A QQ web login with an empty profile must load the account's name and avatar and remain renewable after saving and restoring.
func TestQQWebLoginLoadsAccountProfile(t *testing.T) {
	reads := 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Comm map[string]any `json:"comm"`
			Req0 struct {
				Module, Method string
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Req0.Module != "music.UserInfo.userInfoServer" || body.Req0.Method != "GetLoginUserInfo" {
			t.Errorf("unexpected profile request %s/%s", body.Req0.Module, body.Req0.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		reads++
		if body.Comm["tmeLoginType"] != float64(2) || body.Comm["qq"] != "10001" || body.Comm["authst"] != "fake-musickey" {
			t.Error("profile request did not use the QQ web credential")
		}
		io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"info":{"nick":"Account Listener","headurl":"https://example.test/account-avatar"}}}}`)
	})
	oldHTTP := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = oldHTTP })
	oldRead, oldWrite, oldDelete := readPersistedSession, writePersistedSession, deletePersistedSession
	var stored []byte
	writes := 0
	readPersistedSession = func() ([]byte, error) { return bytes.Clone(stored), nil }
	writePersistedSession = func(b []byte) error { writes++; stored = bytes.Clone(b); return nil }
	deletePersistedSession = func() { t.Error("profile lookup deleted the renewable QQ credential") }
	t.Cleanup(func() {
		readPersistedSession, writePersistedSession, deletePersistedSession = oldRead, oldWrite, oldDelete
	})

	s, err := decodeQQLoginResponse([]byte(qqWebLoginResponse))
	if err != nil {
		t.Fatal(err)
	}
	setSession(s)
	for _, phase := range []string{"after login", "after restore"} {
		profile, err := Account()
		if err != nil || profile == nil || profile.UserID != 10001 || profile.Nickname != "Account Listener" || profile.AvatarURL != "https://example.test/account-avatar" {
			t.Fatalf("profile %s was not loaded from account details: profile=%v err=%v", phase, profile, err)
		}
		if phase == "after restore" {
			break
		}
		if err := SaveSession(); err != nil {
			t.Fatal(err)
		}
		setSession(Session{})
		if ok, err := RestoreSession(); !ok || err != nil {
			t.Fatal("saved QQ web credential could not be restored")
		}
		restored := sessionSnapshot()
		if restored.MusicID != "10001" || restored.MusicKey != s.MusicKey || restored.LoginType != 2 || restored.RefreshKey != "" || restored.RefreshToken != s.RefreshToken || restored.OpenID != s.OpenID || restored.UnionID != s.UnionID || restored.EncryptUin != s.EncryptUin || !restored.canRefresh() {
			t.Fatal("saving the QQ web login lost its token-based renewal material")
		}
	}
	if reads != 2 || writes != 1 {
		t.Fatalf("profile lookup effects: reads=%d saves=%d", reads, writes)
	}
}

// Optional numeric metadata encoded as strings used to reject an otherwise valid QQ credential.
func TestQQLoginAcceptsNumericMetadata(t *testing.T) {
	for _, test := range []struct {
		metadata         string
		created, expires int64
	}{
		{`"musickeyCreateTime":0,"keyExpiresIn":259200`, 0, 259200},
		{`"musickeyCreateTime":"0","keyExpiresIn":"259200"`, 0, 259200},
		{`"musickeyCreateTime":"1760000000","keyExpiresIn":0`, 1760000000, 0},
	} {
		body := `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"private-key","loginType":2,` + test.metadata + `,"nick":null}}}`
		s, err := decodeQQLoginResponse([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if s.MusicID != "10001" || s.CreatedAt != test.created || s.ExpiresIn != test.expires {
			t.Fatal("lost login metadata")
		}
	}
}

// A provider refusal must retain its public business codes instead of collapsing into generic failure.
func TestQQLoginReportsRejectionCodes(t *testing.T) {
	_, err := decodeQQLoginResponse([]byte(`{"code":0,"req":{"code":1000,"data":{"musicid":0,"musickey":""}}}`))
	var response *loginResponseError
	if !errors.As(err, &response) || response.Kind != "rejected" || response.Business != 1000 {
		t.Fatalf("missing rejection metadata: %v", err)
	}
	if !strings.Contains(LoginErrorMessage(loginFailure("qq-login", err)), "0/1000") {
		t.Fatal("provider code hidden")
	}
}

// Missing status or credential fields must not look successful or expose response values in an error.
func TestQQLoginRejectsIncompleteResponse(t *testing.T) {
	for _, test := range []struct{ body, kind string }{
		{`{"req":{"code":0,"data":{}}}`, "status"},
		{`{"code":0,"req":{"code":0}}`, "data"},
		{`{"code":0,"req":{"code":0,"data":{"musickey":"private-key"}}}`, "id"},
		{`{"code":0,"req":{"code":0,"data":{"musicid":10001}}}`, "key"},
		{`{"code":0,"req":{"code":0,"data":{"musicid":10001,"musickey":"private-key","loginType":6}}}`, "type"},
	} {
		_, err := decodeQQLoginResponse([]byte(test.body))
		var response *loginResponseError
		if !errors.As(err, &response) || response.Kind != test.kind {
			t.Fatalf("wanted %s, got %v", test.kind, err)
		}
		if strings.Contains(err.Error(), "private-key") {
			t.Fatal("printed credential")
		}
	}
}

// A TLS close before sending HTTP reconnects once without changing the signed URL.
func TestAuthReconnectsBeforeRequest(t *testing.T) {
	old := authHTTP
	t.Cleanup(func() { authHTTP = old })
	requests := 0
	authHTTP = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "ssl.ptlogin2.graph.qq.com" || r.URL.Query().Get("ptsigx") != "fake" {
			t.Error("changed signed authorization target")
		}
		if requests == 1 {
			return nil, io.EOF
		}
		return &http.Response{StatusCode: 302, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	res, err := authSend("GET", "https://ssl.ptlogin2.graph.qq.com/check_sig?ptsigx=fake", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if requests != 2 {
		t.Fatalf("requests=%d", requests)
	}
}

// Established connections, POSTs and repeated TLS failures must not replay one-time authorization.
func TestAuthDoesNotReplayAuthorization(t *testing.T) {
	for _, test := range []struct {
		name, method string
		connected    bool
		want         int
	}{
		{"connected-get", "GET", true, 1},
		{"oauth-post", "POST", false, 1},
		{"repeated-tls-close", "GET", false, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			old := authHTTP
			t.Cleanup(func() { authHTTP = old })
			requests := 0
			authHTTP = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
				requests++
				if test.connected {
					httptrace.ContextClientTrace(r.Context()).GotConn(httptrace.GotConnInfo{})
				}
				return nil, io.EOF
			})}
			_, err := authSend(test.method, "https://graph.qq.com/oauth2.0/authorize?code=fake", nil, nil)
			if !errors.Is(err, io.EOF) || requests != test.want {
				t.Fatalf("requests=%d err=%v", requests, err)
			}
		})
	}
}

// A WeChat scan retains its cursor and cookie jar, then exchanges the code with the bootstrapped device.
func TestWechatQRLogin(t *testing.T) {
	var imageBytes bytes.Buffer
	_ = png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 37, 37)))
	polls, exchanges := 0, 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Host + r.URL.Path {
		case "api.tencentmusic.com/tme/trpc/proxy":
			if r.Header.Get("method") != "GetQimei" || r.Header.Get("User-Agent") != "QQMusic" {
				t.Error("missing QIMEI headers")
			}
			var body struct {
				Params map[string]string `json:"qimeiParams"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			p := body.Params
			if p["key"] == "" || p["params"] == "" || p["time"] == "" || p["nonce"] == "" || p["sign"] != protocolMD5(p["key"], p["params"], p["time"]+"000", p["nonce"], "ZdJqM15EeO2zWc08", p["extra"]) {
				t.Error("invalid QIMEI signed envelope")
			}
			io.WriteString(w, `{"code":0,"data":"{\"code\":0,\"data\":{\"q16\":\"device-q16\",\"q36\":\"device-q36\"}}"}`)
		case "open.weixin.qq.com/connect/qrconnect":
			if r.URL.Query().Get("appid") != wechatAppID {
				t.Error("incorrect WeChat app id")
			}
			http.SetCookie(w, &http.Cookie{Name: "wx_session", Value: "web-cookie", Domain: "weixin.qq.com", Path: "/"})
			io.WriteString(w, `<img src="/connect/qrcode/uuid-fixture"><a href="https://open.weixin.qq.com/connect/confirm?uuid=uuid-fixture">`)
		case "open.weixin.qq.com/connect/qrcode/uuid-fixture":
			w.Write(imageBytes.Bytes())
		case "lp.open.weixin.qq.com/connect/l/qrconnect":
			if r.Header.Get("Cookie") != "wx_session=web-cookie" {
				t.Error("lost WeChat web cookie")
			}
			if polls >= 2 && r.URL.Query().Get("last") != "404" {
				t.Error("lost scanned cursor")
			}
			statuses := []string{"408", "404", "408", "405"}
			io.WriteString(w, "window.wx_errcode="+statuses[polls]+";window.wx_code='private-code';")
			polls++
		case "u.y.qq.com/cgi-bin/musicu.fcg":
			if r.Header.Get("Cookie") != "" {
				t.Error("WeChat cookie leaked to musicu")
			}
			var body struct {
				Comm map[string]any `json:"comm"`
				Req  struct {
					Module, Method string
					Param          map[string]any
				} `json:"req"`
				Req0 struct{ Module, Method string } `json:"req_0"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Comm["QIMEI"] != "device-q16" || body.Comm["QIMEI36"] != "device-q36" {
				t.Error("lost bootstrapped QIMEI")
			}
			if body.Req0.Method == "GetSession" {
				io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"session":{"uid":123,"sid":"session-sid"}}}}`)
				return
			}
			exchanges++
			if body.Req.Module != "music.login.LoginServer" || body.Req.Method != "Login" || body.Req.Param["strAppid"] != wechatAppID || body.Req.Param["code"] != "private-code" || body.Comm["tmeLoginType"] != float64(1) || body.Comm["uid"] != "123" || body.Comm["sid"] != "session-sid" {
				t.Error("invalid WeChat credential exchange")
			}
			io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":0,"str_musicid":"1152921504606846976","musickey":"wechat-key","openid":"open","refresh_key":"","refresh_token":"token","loginType":1,"nick":"Listener"}}}`)
		default:
			t.Errorf("unexpected request %s%s", r.Host, r.URL.Path)
			w.WriteHeader(500)
		}
	})
	old := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = old })
	setSession(Session{MusicID: "10002", MusicKey: "prior-account", LoginType: 2})
	key, err := WechatQRKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{QRWaiting, QRScanned, QRScanned, QRConfirmed} {
		code, _, _, _, err := QRCheck(key)
		if err != nil || code != want {
			t.Fatalf("check = %d %v, want %d", code, err, want)
		}
	}
	s := sessionSnapshot()
	if exchanges != 1 || !s.valid() || s.MusicID != "1152921504606846976" || s.LoginType != 1 || s.Device == nil || s.RefreshToken != "token" || s.RefreshKey != "" {
		t.Fatal("WeChat session lost its account or refresh data")
	}
	encoded, _ := json.Marshal(s)
	var restored Session
	if json.Unmarshal(encoded, &restored) != nil || !restored.valid() || restored.Device.Qimei != "device-q16" {
		t.Fatal("WeChat session did not survive persistence")
	}
}

// A closed QR must never publish a credential returned by an already running exchange.
func TestCanceledQRDoesNotSignIn(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "lp.open.weixin.qq.com" {
			io.WriteString(w, `window.wx_errcode=405;window.wx_code='code';`)
			return
		}
		close(started)
		<-release
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"str_musicid":"10001","musickey":"key"}}}`)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	authState.Lock()
	authState.qr = map[string]qrLogin{"closed": {expires: time.Now().Add(time.Minute), wechat: &wechatQR{client: authHTTP, uuid: "uuid", ctx: ctx, cancel: cancel, device: &wechatDevice{}}}}
	authState.Unlock()
	result := make(chan int, 1)
	go func() { code, _, _, _, _ := QRCheck("closed"); result <- code }()
	<-started
	CancelQR("closed")
	close(release)
	if <-result != QRExpired || sessionSnapshot().MusicKey != "" {
		t.Fatal("closed QR signed in")
	}
}

// A rejected WeChat key must refresh with all credential fields before retrying the profile read.
func TestWechatAccountRefreshesCredential(t *testing.T) {
	oldWrite := writePersistedSession
	var persisted Session
	writePersistedSession = func(b []byte) error { return json.Unmarshal(b, &persisted) }
	t.Cleanup(func() { writePersistedSession = oldWrite })
	reads, refreshes := 0, 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Comm map[string]any                 `json:"comm"`
			Req  struct{ Param map[string]any } `json:"req"`
			Req0 struct{ Method string }        `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Req0.Method == "GetLoginUserInfo" {
			reads++
			if reads == 1 {
				io.WriteString(w, `{"code":0,"req_0":{"code":1000,"data":{}}}`)
				return
			}
			if body.Comm["authst"] != "new-key" {
				t.Error("profile retry used rejected key")
			}
			io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"info":{"nick":"Listener"}}}}`)
			return
		}
		refreshes++
		if r.Header.Get("Cookie") != "uin=10001; qqmusic_uin=10001; qm_keyst=old-key; qqmusic_key=old-key" {
			t.Error("refresh lost credential cookie")
		}
		p := body.Req.Param
		if body.Comm["tmeLoginType"] != float64(1) || p["openid"] != "open" || p["refresh_token"] != "token" || p["str_musicid"] != "10001" || p["musickey"] != "old-key" || p["unionid"] != "union" || p["refresh_key"] != "" || p["loginMode"] != float64(2) {
			t.Error("incomplete refresh credential")
		}
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"str_musicid":"10001","musickey":"new-key"}}}`)
	})
	old := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = old })
	setSession(Session{MusicID: "10001", MusicKey: "old-key", LoginType: 1, OpenID: "open", RefreshToken: "token", UnionID: "union", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"}})
	p, err := Account()
	if err != nil || p == nil || p.UserID != 10001 || reads != 2 || refreshes != 1 || sessionSnapshot().MusicKey != "new-key" {
		t.Fatalf("refresh failed: profile=%v err=%v reads=%d refreshes=%d", p, err, reads, refreshes)
	}
	if persisted.MusicKey != "new-key" || persisted.RefreshToken != "token" {
		t.Fatal("refreshed credential was not persisted")
	}
}

// Expired WeChat keys with refresh material must survive restore and renew before any account read.
func TestExpiredWechatSessionRestoresAndRefreshes(t *testing.T) {
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Req  struct{ Param map[string]any } `json:"req"`
			Req0 struct{ Method string }        `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Req0.Method == "GetLoginUserInfo" {
			io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"info":{"nick":"renewed"}}}}`)
			return
		}
		if body.Req.Param["loginMode"] != float64(2) {
			t.Error("expired key was not refreshed")
		}
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"str_musicid":"10001","musickey":"renewed-key","refresh_token":"rotated-token"}}}`)
	})
	oldHTTP := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = oldHTTP })
	oldRead, oldWrite := readPersistedSession, writePersistedSession
	stored, _ := json.Marshal(Session{MusicID: "10001", MusicKey: "expired-key", LoginType: 1, CreatedAt: 1, ExpiresIn: 1, OpenID: "open", RefreshToken: "token", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"}})
	readPersistedSession = func() ([]byte, error) { return stored, nil }
	writePersistedSession = func(b []byte) error { stored = bytes.Clone(b); return nil }
	t.Cleanup(func() { readPersistedSession, writePersistedSession = oldRead, oldWrite })
	if ok, err := RestoreSession(); !ok || err != nil {
		t.Fatal("refreshable expired credential was discarded")
	}
	p, err := Account()
	if err != nil || p == nil || p.Nickname != "renewed" {
		t.Fatalf("expired key renewal: %v %v", p, err)
	}
	var renewed Session
	_ = json.Unmarshal(stored, &renewed)
	if !renewed.valid() || renewed.MusicKey != "renewed-key" || renewed.RefreshToken != "rotated-token" || renewed.RefreshKey != "" {
		t.Fatal("rotated credential was not saved")
	}
	readPersistedSession = func() ([]byte, error) {
		return []byte(`{"musicid":"10001","musickey":"old","loginType":3,"musickeyCreateTime":1,"keyExpiresIn":1,"refresh_key":"ignored"}`), nil
	}
	if ok, err := RestoreSession(); ok || err == nil {
		t.Fatal("restored an unsupported credential family")
	}
}
