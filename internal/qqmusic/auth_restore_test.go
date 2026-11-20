package qqmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

func memorySessionStore(t *testing.T, initial []byte) (*[]byte, *int) {
	t.Helper()
	oldRead, oldWrite, oldDelete := readPersistedSession, writePersistedSession, deletePersistedSession
	stored, writes := bytes.Clone(initial), 0
	readPersistedSession = func() ([]byte, error) { return bytes.Clone(stored), nil }
	writePersistedSession = func(b []byte) error { stored = bytes.Clone(b); writes++; return nil }
	deletePersistedSession = func() { t.Error("recovery deleted persisted credentials") }
	prior := sessionSnapshot()
	setSession(Session{})
	t.Cleanup(func() {
		readPersistedSession, writePersistedSession, deletePersistedSession = oldRead, oldWrite, oldDelete
		setSession(prior)
	})
	return &stored, &writes
}

func TestRestoreSessionFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		name, data     string
		err            error
		reload, rescan bool
	}{
		{name: "no Windows credential", err: mygo.ErrSecretNotFound},
		{name: "no dev credential", err: os.ErrNotExist},
		{name: "storage unavailable", err: errors.New("private-storage-value"), reload: true},
		{name: "empty credential", rescan: true},
		{name: "invalid JSON", data: `{"musickey":"private-key"`, rescan: true},
		{name: "expired without refresh", data: `{"musicid":"10001","musickey":"private-key","loginType":2,"musickeyCreateTime":1,"keyExpiresIn":1}`, rescan: true},
		{name: "unsupported family", data: `{"musicid":"10001","musickey":"private-key","loginType":3,"refresh_token":"private-token"}`, rescan: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored, writes := memorySessionStore(t, []byte(tc.data))
			readPersistedSession = func() ([]byte, error) { return []byte(tc.data), tc.err }
			ok, err := RestoreSession()
			if ok || (err != nil) != (tc.reload || tc.rescan) || SessionNeedsReload(err) != tc.reload || SessionNeedsLogin(err) != tc.rescan {
				t.Fatalf("restore classification: ok=%v err=%v reload=%v rescan=%v", ok, err, SessionNeedsReload(err), SessionNeedsLogin(err))
			}
			if *writes != 0 || string(*stored) != tc.data || sessionSnapshot().MusicKey != "" {
				t.Fatal("failed restore modified credentials")
			}
			if err != nil && (strings.Contains(err.Error(), "private-") || strings.Contains(LoginErrorMessage(err), "private-")) {
				t.Fatal("recovery error exposed credential contents")
			}
		})
	}
}

// A web scan has no device. Restore, bootstrap failure, retry, rotation and
// another restart must all preserve the same account and refresh material.
func TestQQWebSessionRecoveryAcrossRestarts(t *testing.T) {
	var bootstraps, sessions, refreshes, reads int
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "api.tencentmusic.com" {
			bootstraps++
			if bootstraps == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			io.WriteString(w, `{"code":0,"data":"{\"code\":0,\"data\":{\"q16\":\"q16\",\"q36\":\"q36\"}}"}`)
			return
		}
		var body struct {
			Comm map[string]any `json:"comm"`
			Req  struct {
				Module, Method string
				Param          map[string]any
			} `json:"req"`
			Req0 struct{ Method string } `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		switch body.Req0.Method {
		case "GetSession":
			sessions++
			if r.Header.Get("Cookie") != "" || body.Comm["QIMEI"] != "q16" {
				t.Error("device initialization used account cookies or lost QIMEI")
			}
			rpcReply(w, map[string]any{"session": map[string]any{"uid": "uid", "sid": "sid"}})
		case "GetLoginUserInfo":
			reads++
			rpcReply(w, map[string]any{"info": map[string]any{"nick": "Listener"}})
		default:
			refreshes++
			p := body.Req.Param
			wantKey, wantToken := "fake-musickey", "fake-refresh-token"
			if refreshes == 2 {
				wantKey, wantToken = "new-key", "rotated-token"
			}
			if body.Req.Module != "music.login.LoginServer" || body.Req.Method != "Login" ||
				body.Comm["tmeLoginType"] != float64(2) || body.Comm["uid"] != "uid" ||
				p["musicid"] != float64(10001) || p["str_musicid"] != nil || p["loginMode"] != float64(2) ||
				p["openid"] != "fake-openid" || p["unionid"] != nil || p["musickey"] != wantKey || p["refresh_token"] != wantToken ||
				p["access_token"] != "" || p["expired_in"] != float64(0) {
				t.Error("QQ renewal changed protocol family or lost saved refresh data")
			}
			io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"new-key","loginType":2,"refresh_token":"rotated-token","refresh_key":"rotated-key"}}}`)
		}
	})
	oldHTTP := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = oldHTTP })
	stored, _ := memorySessionStore(t, nil)
	s, err := decodeQQLoginResponse([]byte(qqWebLoginResponse))
	if err != nil || s.Device != nil {
		t.Fatal("invalid web login fixture")
	}
	setSession(s)
	if err := SaveSession(); err != nil {
		t.Fatal(err)
	}
	expireStored := func() {
		t.Helper()
		var s Session
		if err := json.Unmarshal(*stored, &s); err != nil {
			t.Fatal(err)
		}
		s.CreatedAt, s.ExpiresIn = 1, 1
		*stored, _ = json.Marshal(s)
		setSession(Session{})
		if ok, err := RestoreSession(); !ok || err != nil {
			t.Fatalf("restart failed: %v", err)
		}
	}
	expireStored()
	original := bytes.Clone(*stored)
	if _, err := Account(); err == nil || SessionNeedsLogin(err) {
		t.Fatal("temporary bootstrap failure should offer recovery retry")
	}
	if !bytes.Equal(*stored, original) || sessionSnapshot().Device != nil {
		t.Fatal("failed bootstrap changed the original saved credential")
	}
	for restart := 0; restart < 2; restart++ {
		p, err := Account()
		if err != nil || p == nil || p.UserID != 10001 {
			t.Fatalf("account did not recover: %v", err)
		}
		if restart == 0 {
			expireStored()
		}
	}
	var saved Session
	if err := json.Unmarshal(*stored, &saved); err != nil {
		t.Fatal(err)
	}
	if bootstraps != 2 || sessions != 1 || refreshes != 2 || reads != 2 ||
		saved.MusicKey != "new-key" || saved.Device == nil || saved.Device.Qimei != "q16" || saved.Device.UID != "uid" ||
		saved.RefreshKey != "rotated-key" || saved.RefreshToken != "rotated-token" {
		t.Fatalf("unexpected recovery effects: bootstrap=%d sessions=%d refresh=%d reads=%d", bootstraps, sessions, refreshes, reads)
	}
}

func TestConcurrentQQRefreshAndAccountReplacement(t *testing.T) {
	for _, scenario := range []string{"concurrent", "switch account", "logout"} {
		t.Run(scenario, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					close(started)
				}
				<-release
				io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"new-key","loginType":2}}}`)
			})
			_, writes := memorySessionStore(t, nil)
			deleted := false
			deletePersistedSession = func() { deleted = true }
			setSession(Session{MusicID: "10001", MusicKey: "expired", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, RefreshToken: "token", Device: &wechatDevice{}})
			callers := 8
			if scenario == "logout" {
				callers = 1
			}
			var wg sync.WaitGroup
			for range callers {
				wg.Go(func() {
					if _, err := activeSession(); err != nil {
						t.Error(err)
					}
				})
			}
			<-started
			switch scenario {
			case "switch account":
				setSession(Session{MusicID: "10002", MusicKey: "replacement", LoginType: 2})
			case "logout":
				Logout()
			}
			close(release)
			wg.Wait()
			if calls.Load() != 1 {
				t.Fatalf("concurrent calls refreshed %d times", calls.Load())
			}
			s := sessionSnapshot()
			switch scenario {
			case "concurrent":
				if *writes != 1 || deleted || s.MusicKey != "new-key" {
					t.Fatal("concurrent calls did not share the renewed credential")
				}
			case "switch account":
				if *writes != 0 || deleted || s.MusicID != "10002" {
					t.Fatal("old renewal overwrote the replacement account")
				}
			case "logout":
				if *writes != 0 || !deleted || s != (Session{}) {
					t.Fatal("old renewal undid logout")
				}
			}
		})
	}
}

func TestQQRefreshRetriesPersistenceWithoutReusingRotatedToken(t *testing.T) {
	refreshes := 0
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		refreshes++
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"new-key","loginType":2,"refresh_token":"rotated-token"}}}`)
	})
	original := Session{MusicID: "10001", MusicKey: "expired", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, RefreshToken: "old-token", Device: &wechatDevice{}}
	b, _ := json.Marshal(original)
	stored, writes := memorySessionStore(t, b)
	setSession(original)
	persist := writePersistedSession
	attempts := 0
	writePersistedSession = func(b []byte) error {
		attempts++
		if attempts <= 2 {
			return errors.New("private-storage-error")
		}
		return persist(b)
	}
	for i := 0; i < 2; i++ {
		_, err := activeSession()
		if err == nil || !strings.Contains(SessionErrorMessage(err), "S104") || SessionNeedsLogin(err) {
			t.Fatalf("persistence failure not reported: %v", err)
		}
		if !bytes.Equal(*stored, b) || sessionSnapshot().RefreshToken != "rotated-token" || refreshes != 1 {
			t.Fatal("persistence failure lost the rotated token or overwrote the old credential")
		}
	}
	if _, err := activeSession(); err != nil || refreshes != 1 || *writes != 1 {
		t.Fatalf("retry did not save the rotated credential: %v", err)
	}
	setSession(Session{})
	if ok, err := RestoreSession(); !ok || err != nil || sessionSnapshot().RefreshToken != "rotated-token" {
		t.Fatal("successfully retried persistence did not survive restart")
	}
}

type testSessionSecrets struct {
	entry    []byte
	writeErr error
}

func (s *testSessionSecrets) Get(string) ([]byte, error) {
	if s.entry == nil {
		return nil, mygo.ErrSecretNotFound
	}
	return bytes.Clone(s.entry), nil
}

func (s *testSessionSecrets) Set(_ string, b []byte) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	if len(b) > mygo.MaxSecretSize {
		return errors.New("test credential store limit exceeded")
	}
	s.entry = bytes.Clone(b)
	return nil
}

func (s *testSessionSecrets) Delete(string) error {
	s.entry = nil
	return nil
}

func testSessionSecretDir(t *testing.T, dir string) {
	t.Helper()
	previous := sessionSecretDir
	sessionSecretDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { sessionSecretDir = previous })
}

func TestOversizedQQRenewalSurvivesRestart(t *testing.T) {
	original := Session{MusicID: "10001", MusicKey: "old-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, RefreshToken: "old-token", Device: &wechatDevice{}}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	original.RefreshToken += strings.Repeat("x", mygo.MaxSecretSize-len(b)-1)
	b, err = json.Marshal(original)
	if err != nil || len(b) != mygo.MaxSecretSize-1 {
		t.Fatal("fixture must fit just below the credential store limit")
	}
	dir := filepath.Join(t.TempDir(), "qqmusic-session.d")
	testSessionSecretDir(t, dir)
	store := &testSessionSecrets{entry: b}
	memorySessionStore(t, nil)
	readPersistedSession = func() ([]byte, error) { return readSessionSecret(store, sessionSecretDir) }
	writePersistedSession = func(b []byte) error { return writeSessionSecret(store, b) }
	refreshes := 0
	rotatedToken := "new" + original.RefreshToken[3:]
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		refreshes++
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"new-key","loginType":2,"refresh_token":"`+rotatedToken+`","access_token":"new-access-token","expired_at":1760000000}}}`)
	})
	if ok, err := RestoreSession(); !ok || err != nil {
		t.Fatalf("legacy credential did not restore: %v", err)
	}
	originalPublish := publishSessionFile
	t.Cleanup(func() { publishSessionFile = originalPublish })
	publishErr := errors.New("test ciphertext publication failed")
	publishSessionFile = func(_, _ string) error { return publishErr }
	if _, err := activeSession(); !errors.Is(err, publishErr) || !bytes.Equal(store.entry, b) {
		t.Fatalf("ciphertext publication failure replaced the credential: %v", err)
	}
	publishSessionFile = originalPublish
	store.writeErr = errors.New("test credential store write failed")
	if _, err := activeSession(); !errors.Is(err, store.writeErr) {
		t.Fatalf("publication failure was not reported: %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 || !bytes.Equal(store.entry, b) {
		t.Fatal("failed save replaced the previous credential or left an encrypted file")
	}
	store.writeErr = nil
	if _, err := activeSession(); err != nil || refreshes != 1 {
		t.Fatalf("saving the rotated credential again should not contact the server: %v", err)
	}
	files, err = os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("renewal did not publish exactly one encrypted file")
	}
	ciphertext, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil || bytes.Contains(ciphertext, []byte("new-access-token")) || bytes.Contains(store.entry, []byte("new-access-token")) {
		t.Fatal("renewal did not protect the new access token")
	}
	setSession(Session{})
	if ok, err := RestoreSession(); !ok || err != nil {
		t.Fatalf("oversized renewal did not restore after restart: %v", err)
	}
	if saved := sessionSnapshot(); saved.MusicKey != "new-key" || saved.RefreshToken != rotatedToken || saved.AccessToken != "new-access-token" || saved.ExpiredAt != 1760000000 {
		t.Fatal("restored credential lost renewed fields")
	}
	if err := deleteSessionSecret(store); err != nil || store.entry != nil {
		t.Fatalf("logout did not remove the encrypted session reference: %v", err)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
		t.Fatal("logout did not remove the encrypted session file")
	}
}

func TestSessionPersistenceSizeAndRedactedLogs(t *testing.T) {
	t.Setenv("MYGO_ENV", "production")
	b := bytes.Repeat([]byte("x"), maxSessionSize+1)
	if err := writePersistedSession(b); !errors.Is(err, errSessionTooLarge) {
		t.Fatalf("oversized credential was not rejected before system storage: %v", err)
	}
	var output bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(oldWriter)
	s := Session{MusicID: "private-id", MusicKey: "private-key", RefreshToken: "private-token"}
	err := sessionFailure("save", s, errors.New("private-storage-error"))
	sessionFailure("refresh", s, &loginResponseError{Kind: "rejected", Business: 1000})
	if strings.Contains(output.String(), "private-") || strings.Contains(err.Error(), "private-") ||
		!strings.Contains(output.String(), "refreshToken=true") || !strings.Contains(output.String(), "code=1000") {
		t.Fatal("session diagnostics exposed credentials or omitted safe status fields")
	}
}

func TestRestoreSessionDoesNotOverrideLogoutOrAccountChange(t *testing.T) {
	for _, logout := range []bool{false, true} {
		t.Run(map[bool]string{false: "switch account", true: "logout"}[logout], func(t *testing.T) {
			b, _ := json.Marshal(Session{MusicID: "10001", MusicKey: "old-key", LoginType: 2, RefreshToken: "token"})
			memorySessionStore(t, b)
			started, release := make(chan struct{}), make(chan struct{})
			readPersistedSession = func() ([]byte, error) {
				close(started)
				<-release
				return bytes.Clone(b), nil
			}
			done := make(chan error, 1)
			go func() { _, err := RestoreSession(); done <- err }()
			<-started
			if logout {
				deletePersistedSession = func() {}
				Logout()
			} else {
				setSession(Session{MusicID: "10002", MusicKey: "replacement", LoginType: 2})
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			s := sessionSnapshot()
			if logout && s.MusicID != "" || !logout && s.MusicID != "10002" {
				t.Fatal("delayed restore resurrected the old account")
			}
		})
	}
}

func TestSaveSessionCannotWriteBackAfterLogout(t *testing.T) {
	stored, _ := memorySessionStore(t, nil)
	setSession(Session{MusicID: "10001", MusicKey: "key", LoginType: 2})
	started, release := make(chan struct{}), make(chan struct{})
	persist := writePersistedSession
	writePersistedSession = func(b []byte) error {
		close(started)
		<-release
		return persist(b)
	}
	deletePersistedSession = func() { *stored = nil }
	saved, loggedOut := make(chan error, 1), make(chan struct{})
	go func() { saved <- SaveSession() }()
	<-started
	go func() { Logout(); close(loggedOut) }()
	close(release)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	<-loggedOut
	if len(*stored) != 0 || sessionSnapshot().MusicKey != "" {
		t.Fatal("an in-flight save restored a logged-out credential")
	}
}

func TestQQRefreshRejectsDifferentAccount(t *testing.T) {
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10002","musickey":"other-key","loginType":2}}}`)
	})
	_, writes := memorySessionStore(t, nil)
	original := Session{MusicID: "10001", MusicKey: "expired", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, RefreshToken: "token", Device: &wechatDevice{}}
	setSession(original)
	if _, err := activeSession(); err == nil || SessionNeedsLogin(err) || *writes != 0 || sessionSnapshot() != original {
		t.Fatal("renewal response for another account replaced the saved credential")
	}
}

// Expired QQ sessions used to be discarded even when their saved refresh material could renew the key.
func TestExpiredQQSessionRestoresAndRefreshes(t *testing.T) {
	for _, tc := range []struct {
		name, refreshKey, refreshToken, accessToken string
		expiredAt                                   int64
	}{
		{name: "refresh key", refreshKey: "saved-refresh-key", accessToken: "saved-access-token", expiredAt: 1760000000},
		{name: "legacy refresh token", refreshToken: "saved-refresh-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, refreshes := 0, 0
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Comm map[string]any                 `json:"comm"`
					Req  struct{ Param map[string]any } `json:"req"`
					Req0 struct{ Method string }        `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if body.Req0.Method == "GetLoginUserInfo" {
					reads++
					if body.Comm["authst"] != "renewed-key" || body.Comm["tmeLoginType"] != float64(2) {
						t.Error("profile read did not use the renewed QQ credential")
					}
					io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"info":{}}}}`)
					return
				}
				refreshes++
				if body.Comm["tmeLoginType"] != float64(2) {
					t.Error("QQ renewal used the WeChat credential family")
				}
				p := body.Req.Param
				if p["musicid"] != float64(10001) || p["str_musicid"] != nil || p["unionid"] != nil ||
					p["access_token"] != tc.accessToken || p["expired_in"] != float64(tc.expiredAt) ||
					p["musickey"] != "expired-key" || p["openid"] != "saved-openid" ||
					p["refresh_key"] != tc.refreshKey || p["refresh_token"] != tc.refreshToken || p["loginMode"] != float64(2) {
					t.Error("QQ renewal lost the saved credential material")
				}
				io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"renewed-key","loginType":2,"refresh_token":"rotated-token"}}}`)
			})
			oldHTTP := httpClient
			httpClient = authHTTP
			t.Cleanup(func() { httpClient = oldHTTP })
			original := Session{
				MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1,
				OpenID: "saved-openid", UnionID: "saved-unionid", RefreshKey: tc.refreshKey, RefreshToken: tc.refreshToken,
				AccessToken: tc.accessToken, ExpiredAt: tc.expiredAt,
				Nickname: "Listener", AvatarURL: "https://example.test/avatar", EncryptUin: "saved-encrypt-uin",
				Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"},
			}
			stored, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			oldRead, oldWrite, oldDelete := readPersistedSession, writePersistedSession, deletePersistedSession
			writes, deleted := 0, false
			readPersistedSession = func() ([]byte, error) { return bytes.Clone(stored), nil }
			writePersistedSession = func(b []byte) error { writes++; stored = bytes.Clone(b); return nil }
			deletePersistedSession = func() { deleted = true }
			t.Cleanup(func() {
				readPersistedSession, writePersistedSession, deletePersistedSession = oldRead, oldWrite, oldDelete
			})
			if ok, err := RestoreSession(); !ok || err != nil {
				t.Fatal("refreshable expired QQ credential was discarded")
			}
			profile, err := Account()
			if err != nil || profile == nil || profile.UserID != 10001 || profile.Nickname != original.Nickname {
				t.Fatalf("QQ account did not recover: profile=%v err=%v", profile, err)
			}
			if reads != 1 || refreshes != 1 || writes != 1 || deleted {
				t.Fatalf("recovery effects: profile reads=%d refreshes=%d writes=%d deleted=%v", reads, refreshes, writes, deleted)
			}
			var saved Session
			if err := json.Unmarshal(stored, &saved); err != nil {
				t.Fatal(err)
			}
			for _, got := range []Session{sessionSnapshot(), saved} {
				if got.AccessToken != tc.accessToken || got.ExpiredAt != tc.expiredAt {
					t.Error("QQ renewal lost OAuth metadata omitted by the response")
				}
				if got.MusicKey != "renewed-key" || got.RefreshToken != "rotated-token" || got.LoginType != 2 || got.MusicID != original.MusicID || got.RefreshKey != original.RefreshKey || got.OpenID != original.OpenID || got.UnionID != original.UnionID || got.Nickname != original.Nickname || got.AvatarURL != original.AvatarURL || got.EncryptUin != original.EncryptUin || got.Device == nil || *got.Device != *original.Device {
					t.Fatal("QQ renewal did not save the new key and token while retaining omitted fields")
				}
			}
		})
	}
}

// A failed renewal must retain the original persisted material so the same expired QQ session can retry.
func TestQQRefreshFailurePreservesRecoverableSession(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		status         int
	}{
		{name: "business 1000", response: `{"code":0,"req":{"code":1000}}`},
		{name: "business 10006", response: `{"code":0,"req":{"code":10006}}`},
		{name: "HTTP 503", status: http.StatusServiceUnavailable},
		{name: "malformed response", response: `not JSON`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refreshes, reads := 0, 0
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Req0 struct{ Method string } `json:"req_0"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body.Req0.Method == "GetLoginUserInfo" {
					reads++
					io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"info":{}}}}`)
					return
				}
				refreshes++
				if refreshes == 1 {
					if tc.status != 0 {
						w.WriteHeader(tc.status)
					}
					io.WriteString(w, tc.response)
					return
				}
				io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"recovered-key","loginType":2}}}`)
			})
			oldHTTP := httpClient
			httpClient = authHTTP
			t.Cleanup(func() { httpClient = oldHTTP })
			stored, err := json.Marshal(Session{
				MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1,
				OpenID: "saved-openid", RefreshToken: "saved-token", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"},
			})
			if err != nil {
				t.Fatal(err)
			}
			originalBytes := bytes.Clone(stored)
			oldRead, oldWrite, oldDelete := readPersistedSession, writePersistedSession, deletePersistedSession
			writes, deleted := 0, false
			readPersistedSession = func() ([]byte, error) { return bytes.Clone(stored), nil }
			writePersistedSession = func(b []byte) error { writes++; stored = bytes.Clone(b); return nil }
			deletePersistedSession = func() { deleted = true }
			t.Cleanup(func() {
				readPersistedSession, writePersistedSession, deletePersistedSession = oldRead, oldWrite, oldDelete
			})
			if ok, err := RestoreSession(); !ok || err != nil {
				t.Fatal("expired QQ session was not restored for renewal")
			}
			original := sessionSnapshot()
			if profile, err := Account(); err == nil || profile != nil {
				t.Fatalf("failed renewal was hidden: profile=%v err=%v", profile, err)
			}
			if deleted || writes != 0 || !bytes.Equal(stored, originalBytes) || sessionSnapshot() != original || reads != 0 {
				t.Fatal("failed renewal changed the recoverable session or queried the expired account")
			}
			profile, err := Account()
			if err != nil || profile == nil || profile.UserID != 10001 || refreshes != 2 || reads != 1 || writes != 1 || deleted || sessionSnapshot().MusicKey != "recovered-key" {
				t.Fatalf("same session could not retry: err=%v refreshes=%d reads=%d writes=%d deleted=%v", err, refreshes, reads, writes, deleted)
			}
		})
	}
}

// A profile rejection after successful renewal must stop after one retry without deleting recoverable material.
func TestQQAccountRetryDoesNotDiscardRenewedSession(t *testing.T) {
	reads, refreshes := 0, 0
	// Guard the fixture against an accidental retry loop as well as checking the final request counts.
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Comm map[string]any          `json:"comm"`
			Req0 struct{ Method string } `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Req0.Method == "GetLoginUserInfo" {
			reads++
			if reads == 2 && body.Comm["authst"] != "renewed-key" {
				t.Error("profile retry used the old key")
			}
			io.WriteString(w, `{"code":0,"req_0":{"code":1000,"data":{}}}`)
			return
		}
		refreshes++
		if refreshes > 1 {
			t.Error("account rejection started a second renewal")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"renewed-key","loginType":2}}}`)
	})
	oldHTTP := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = oldHTTP })
	oldWrite, oldDelete := writePersistedSession, deletePersistedSession
	writes, deleted := 0, false
	var persisted Session
	writePersistedSession = func(b []byte) error { writes++; return json.Unmarshal(b, &persisted) }
	deletePersistedSession = func() { deleted = true }
	t.Cleanup(func() { writePersistedSession, deletePersistedSession = oldWrite, oldDelete })
	setSession(Session{MusicID: "10001", MusicKey: "old-key", LoginType: 2, RefreshToken: "saved-token", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"}})
	profile, err := Account()
	if err == nil || profile != nil || reads != 2 || refreshes != 1 || writes != 1 || deleted {
		t.Fatalf("repeated profile rejection: err=%v reads=%d refreshes=%d writes=%d deleted=%v", err, reads, refreshes, writes, deleted)
	}
	if sessionSnapshot().MusicKey != "renewed-key" || persisted.MusicKey != "renewed-key" || persisted.RefreshToken != "saved-token" {
		t.Fatal("repeated profile rejection discarded the renewed key or saved refresh material")
	}
}

// A renewal response from the wrong credential family must not replace the saved QQ session.
func TestQQRefreshRejectsWrongCredentialFamily(t *testing.T) {
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","str_musicid":"10001","musickey":"wrong-family-key","loginType":1}}}`)
	})
	oldHTTP := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = oldHTTP })
	oldWrite, oldDelete := writePersistedSession, deletePersistedSession
	writes, deleted := 0, false
	writePersistedSession = func([]byte) error { writes++; return nil }
	deletePersistedSession = func() { deleted = true }
	t.Cleanup(func() { writePersistedSession, deletePersistedSession = oldWrite, oldDelete })
	original := Session{MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, RefreshToken: "saved-token", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"}}
	setSession(original)
	if profile, err := Account(); err == nil || profile != nil {
		t.Fatalf("wrong credential family was accepted: profile=%v err=%v", profile, err)
	}
	if writes != 0 || deleted || sessionSnapshot() != original {
		t.Fatal("wrong credential family overwrote or deleted the original QQ session")
	}
}

// Once global session renewal fails, SongURL must immediately return a login prompt without
// repeating refresh network requests for every uncached track, while Account() can still retry.
func TestSongURLShortCircuitsOnFailedSessionRenewal(t *testing.T) {
	refreshes, vkeys, allowSuccess := 0, 0, false
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Req struct {
				Method string
			} `json:"req"`
			Req0 struct {
				Module, Method string
			} `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case body.Req.Method == "Login":
			refreshes++
			if !allowSuccess {
				io.WriteString(w, `{"code":0,"req":{"code":1000}}`)
				return
			}
			io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"recovered-key","loginType":2,"refresh_token":"rotated-token"}}}`)
		case body.Req0.Method == "GetLoginUserInfo":
			io.WriteString(w, `{"code":0,"req_0":{"code":0,"data":{"info":{}}}}`)
		case body.Req0.Method == "UrlGetVkey" || body.Req0.Method == "CgiGetVkey":
			vkeys++
			rpcReply(w, map[string]any{"sip": []string{"https://dl.stream.qqmusic.qq.com/"}, "midurlinfo": []any{map[string]any{"songmid": "songMid1", "purl": "M500file.mp3"}}})
		default:
			t.Errorf("unexpected RPC %+v", body)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	oldHTTP, oldWrite, oldDelete := httpClient, writePersistedSession, deletePersistedSession
	httpClient = authHTTP
	writePersistedSession = func([]byte) error { return nil }
	deletePersistedSession = func() {}
	t.Cleanup(func() {
		httpClient, writePersistedSession, deletePersistedSession = oldHTTP, oldWrite, oldDelete
		setSession(Session{})
	})
	RememberTracks([]Track{
		{ID: 7001, MID: "songMid1", MediaMID: "mediaMid1"},
		{ID: 7002, MID: "songMid2", MediaMID: "mediaMid2"},
	})
	setSession(Session{
		MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1,
		OpenID: "saved-openid", RefreshToken: "saved-token", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36"},
	})

	// Failure at startup Account() puts global session into renewal-failed state.
	if profile, err := Account(); err == nil || profile != nil {
		t.Fatalf("expected failed startup Account(), got profile=%v err=%v", profile, err)
	}
	if refreshes != 1 {
		t.Fatalf("startup Account() refreshes = %d, want 1", refreshes)
	}

	// Multiple SongURL calls must not trigger any additional refresh or vkey requests.
	for _, id := range []int64{7001, 7002, 7001} {
		if _, err := SongURL(id, "standard"); !errors.Is(err, ErrLoginRequired) {
			t.Fatalf("SongURL(%d) = %v, want ErrLoginRequired", id, err)
		}
	}
	if refreshes != 1 || vkeys != 0 {
		t.Fatalf("SongURL triggered unexpected requests after failed renewal: refreshes=%d vkeys=%d", refreshes, vkeys)
	}

	// Explicit Account() recovery retries and clears the renewal-failed state on success.
	allowSuccess = true
	profile, err := Account()
	if err != nil || profile == nil || refreshes != 2 {
		t.Fatalf("Account() recovery failed: profile=%v err=%v refreshes=%d", profile, err, refreshes)
	}
	stream, err := SongURL(7001, "standard")
	if err != nil || stream == nil || vkeys != 1 {
		t.Fatalf("SongURL after recovery failed: stream=%+v err=%v vkeys=%d", stream, err, vkeys)
	}
}

// Expired sessions must renew before a library write, radio read or search instead of asking to sign in or becoming anonymous.
func TestOperationsRenewExpiredSession(t *testing.T) {
	for _, operation := range []string{"library", "radio", "search"} {
		t.Run(operation, func(t *testing.T) {
			refreshes, calls, saves := 0, 0, 0
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Comm map[string]any          `json:"comm"`
					Req  struct{ Method string } `json:"req"`
					Req0 struct{ Method string } `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Req.Method == "Login" {
					refreshes++
					io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"renewed-key","loginType":2}}}`)
					return
				}
				calls++
				if body.Comm["authst"] != "renewed-key" || body.Comm["tmeLoginType"] != float64(2) || refreshes != 1 {
					t.Error("operation used an expired or anonymous credential")
				}
				switch body.Req0.Method {
				case "CancelFavPlaylist":
					rpcReply(w, map[string]any{"result": 0, "v_failedPlaylistId": []int64{}})
				case "get_radio_track":
					rpcReply(w, map[string]any{"tracks": []any{map[string]any{"id": 101, "mid": "songMid", "title": "Song"}}})
				case "DoSearchForQQMusicLite":
					rpcReply(w, map[string]any{"body": map[string]any{"item_song": []any{map[string]any{"id": 101, "mid": "songMid", "title": "Song"}}}, "meta": map[string]any{"sum": 1}})
				default:
					t.Errorf("unexpected operation %s", body.Req0.Method)
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			oldHTTP, oldSearches, oldWrite := httpClient, searches, writePersistedSession
			httpClient = authHTTP
			searches = newSearchEngine()
			searches.interval = 0
			writePersistedSession = func([]byte) error { saves++; return nil }
			t.Cleanup(func() { httpClient, searches, writePersistedSession = oldHTTP, oldSearches, oldWrite })
			setSession(Session{MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, OpenID: "open", RefreshToken: "token", Device: &wechatDevice{}})
			switch operation {
			case "library":
				if err := UnfavoritePlaylist(901); err != nil {
					t.Fatal(err)
				}
			case "radio":
				tracks, err := PersonalFM(FMDefault)
				if err != nil || len(tracks) != 1 || tracks[0].ID != 101 {
					t.Fatalf("radio did not recover: tracks=%d err=%v", len(tracks), err)
				}
			case "search":
				result, err := SearchPageSource("Song", SearchSongs, 20, 0, SearchLite)
				if err != nil || result.Len() != 1 || result.Songs[0].ID != 101 {
					t.Fatalf("search did not recover: rows=%d err=%v", result.Len(), err)
				}
			}
			if refreshes != 1 || calls != 1 || saves != 1 {
				t.Fatalf("operation repeated or skipped: refreshes=%d calls=%d saves=%d", refreshes, calls, saves)
			}
		})
	}
}

// Canceling a search must also cancel renewal or device bootstrap and retain the original expired session.
func TestSearchCancellationDuringRenewal(t *testing.T) {
	for _, stage := range []string{"refresh", "device bootstrap", "canceled before renewal"} {
		t.Run(stage, func(t *testing.T) {
			started, aborted := make(chan struct{}), make(chan struct{})
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				<-r.Context().Done()
				close(aborted)
			})
			original := Session{MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, OpenID: "open", RefreshToken: "token", Device: &wechatDevice{}}
			if stage == "device bootstrap" {
				original.Device = nil
			}
			setSession(original)
			oldWrite := writePersistedSession
			saved := false
			writePersistedSession = func([]byte) error { saved = true; return nil }
			t.Cleanup(func() { writePersistedSession = oldWrite })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if stage == "canceled before renewal" {
				sessionRefreshGate <- struct{}{}
				defer func() { <-sessionRefreshGate }()
				cancel()
			}
			done := make(chan error, 1)
			go func() { _, err := SearchPageContext(ctx, "Song", SearchSongs, 20, 0, SearchLite); done <- err }()
			if stage != "canceled before renewal" {
				select {
				case <-started:
				case <-time.After(2 * time.Second):
					t.Fatal("renewal did not start")
				}
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation returned %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("search cancellation waited for authentication timeout")
			}
			if stage != "canceled before renewal" {
				select {
				case <-aborted:
				case <-time.After(2 * time.Second):
					t.Fatal("authentication HTTP request was not canceled")
				}
			}
			if saved || sessionSnapshot() != original {
				t.Fatal("cancellation changed the recoverable session")
			}
		})
	}
}
