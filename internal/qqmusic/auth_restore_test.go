package qqmusic

import (
	"bytes"
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
			wantToken := "fake-refresh-token"
			if refreshes == 2 {
				wantToken = "rotated-token"
			}
			if body.Req.Module != "music.login.LoginServer" || body.Req.Method != "Login" ||
				body.Comm["tmeLoginType"] != float64(2) || body.Comm["uid"] != "uid" ||
				p["musicid"] != float64(10001) || p["str_musicid"] != nil || p["loginMode"] != float64(2) ||
				p["openid"] != "fake-openid" || p["unionid"] != nil || p["refresh_token"] != wantToken ||
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
		saved.Device == nil || saved.Device.UID != "uid" || saved.RefreshKey != "rotated-key" || saved.RefreshToken != "rotated-token" {
		t.Fatalf("unexpected recovery effects: bootstrap=%d sessions=%d refresh=%d reads=%d", bootstraps, sessions, refreshes, reads)
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

func TestConcurrentQQRefreshAndAccountReplacement(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent", true: "switch account"}[replace], func(t *testing.T) {
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
			setSession(Session{MusicID: "10001", MusicKey: "expired", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, RefreshToken: "token", Device: &wechatDevice{}})
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Go(func() {
					if _, err := activeSession(); err != nil {
						t.Error(err)
					}
				})
			}
			<-started
			if replace {
				setSession(Session{MusicID: "10002", MusicKey: "replacement", LoginType: 2})
			}
			close(release)
			wg.Wait()
			if calls.Load() != 1 {
				t.Fatalf("concurrent calls refreshed %d times", calls.Load())
			}
			if replace && (*writes != 0 || sessionSnapshot().MusicID != "10002") {
				t.Fatal("old renewal overwrote the replacement account")
			}
			if !replace && (*writes != 1 || sessionSnapshot().MusicKey != "new-key") {
				t.Fatal("concurrent calls did not share the renewed credential")
			}
		})
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
