package qqmusic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// Expired QQ sessions used to be discarded even when their saved refresh material could renew the key.
func TestExpiredQQSessionRestoresAndRefreshes(t *testing.T) {
	for _, tc := range []struct {
		name, refreshKey, refreshToken string
	}{
		{name: "refresh key", refreshKey: "saved-refresh-key"},
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
				if p["musicid"] != "10001" || p["str_musicid"] != "10001" || p["musickey"] != "expired-key" || p["openid"] != "saved-openid" || p["unionid"] != "saved-unionid" || p["refresh_key"] != tc.refreshKey || p["refresh_token"] != tc.refreshToken || p["loginMode"] != float64(2) {
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
			if !RestoreSession() {
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
			if !RestoreSession() {
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
