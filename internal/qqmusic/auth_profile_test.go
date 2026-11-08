package qqmusic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

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
		if !RestoreSession() {
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
