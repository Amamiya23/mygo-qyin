package qqmusic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

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

// Legacy QQ sessions have no saved device, so their first renewal must finish device initialization and persist it.
func TestLegacyQQRefreshWithoutDevice(t *testing.T) {
	bootstraps, sessions, refreshes := 0, 0, 0
	var saved Session
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "api.tencentmusic.com" {
			bootstraps++
			io.WriteString(w, `{"code":0,"data":"{\"code\":0,\"data\":{\"q16\":\"q16\",\"q36\":\"q36\"}}"}`)
			return
		}
		var body struct {
			Comm map[string]any `json:"comm"`
			Req  struct {
				Method string
				Param  map[string]any
			} `json:"req"`
			Req0 struct{ Method string } `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Req0.Method == "GetSession" {
			sessions++
			if r.Header.Get("Cookie") != "" {
				t.Error("device bootstrap sent the expired session cookie")
			}
			rpcReply(w, map[string]any{"session": map[string]any{"uid": "device-uid", "sid": "device-sid"}})
			return
		}
		refreshes++
		if body.Req.Method != "Login" || body.Comm["tmeLoginType"] != float64(2) || body.Comm["uid"] != "device-uid" || body.Req.Param["musickey"] != "expired-key" || body.Req.Param["refresh_token"] != "token" {
			t.Error("legacy QQ refresh lost its credential family or saved material")
		}
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"new-key","loginType":2}}}`)
	})
	oldHTTP, oldWrite := httpClient, writePersistedSession
	httpClient = authHTTP
	writePersistedSession = func(b []byte) error { return json.Unmarshal(b, &saved) }
	t.Cleanup(func() { httpClient, writePersistedSession = oldHTTP, oldWrite })
	setSession(Session{MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, OpenID: "open", RefreshToken: "token"})
	s, err := activeSession()
	if err != nil || s.MusicKey != "new-key" || saved.Device == nil || saved.Device.Qimei != "q16" || saved.Device.UID != "device-uid" || bootstraps != 1 || sessions != 1 || refreshes != 1 {
		t.Fatalf("legacy QQ recovery failed: err=%v bootstrap=%d sessions=%d refreshes=%d", err, bootstraps, sessions, refreshes)
	}
}

// Signing out while a renewal is in flight must not restore or persist the old account when the response arrives.
func TestLogoutDuringSessionRefresh(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		io.WriteString(w, `{"code":0,"req":{"code":0,"data":{"musicid":"10001","musickey":"new-key","loginType":2}}}`)
	})
	oldWrite, oldDelete := writePersistedSession, deletePersistedSession
	saved, deleted := false, false
	writePersistedSession = func([]byte) error { saved = true; return nil }
	deletePersistedSession = func() { deleted = true }
	t.Cleanup(func() { writePersistedSession, deletePersistedSession = oldWrite, oldDelete })
	setSession(Session{MusicID: "10001", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1, RefreshToken: "token", Device: &wechatDevice{}})
	done := make(chan error, 1)
	go func() { _, err := activeSession(); done <- err }()
	<-started
	Logout()
	close(release)
	if err := <-done; err != nil || saved || !deleted || sessionSnapshot().MusicKey != "" {
		t.Fatalf("renewal undid logout: err=%v saved=%v deleted=%v", err, saved, deleted)
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
