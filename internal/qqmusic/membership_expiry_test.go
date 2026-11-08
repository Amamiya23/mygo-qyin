package qqmusic

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The date query returns green and super membership dates separately.
func TestUserMembershipDates(t *testing.T) {
	fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
		return map[string]any{"identity": map[string]any{"overdate": "2030-02-03", "HugeVipEnd": "2032-04-05"}}
	})
	green, super, err := UserMembershipDates()
	if err != nil || green != "2030-02-03" || super != "2032-04-05" {
		t.Fatalf("dates = %q, %q, %v", green, super, err)
	}
}

// Membership expiry selects dates for the requested tier without confusing green and super dates.
func TestUserMembershipExpiry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		level    Membership
		identity map[string]any
		userinfo map[string]any
		want     string
	}{
		{"green prefers overdate", MembershipGreen, map[string]any{"overdate": "2030-02-03 12:34:56", "LMEnd": "2031-03-04", "HugeVipEnd": "2032-04-05"}, map[string]any{"expire": 1893456000}, "2030-02-03"},
		{"green does not use milestone date", MembershipGreen, map[string]any{"overdate": "2030-02-30", "LMEnd": "2031-03-04"}, nil, ""},
		{"green does not use unverified userinfo date", MembershipGreen, nil, map[string]any{"expire": 1893456000}, ""},
		{"super uses HugeVipEnd", MembershipSuper, map[string]any{"overdate": "2030-02-03", "HugeVipEnd": "2032-04-05 12:00:00"}, nil, "2032-04-05"},
		{"super does not use green date", MembershipSuper, map[string]any{"overdate": "2030-02-03", "HugeVipEnd": "0000-00-00"}, map[string]any{"expire": 1893456000}, ""},
		{"invalid and missing dates", MembershipGreen, map[string]any{"overdate": "2030-02-30", "LMEnd": "not a date"}, map[string]any{"expire": -1}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
				if module != "VipLogin.VipLoginInter" || method != "vip_login_base" || param == nil || len(param) != 0 {
					t.Errorf("unexpected request %s/%s %v", module, method, param)
				}
				return map[string]any{"identity": tc.identity, "userinfo": tc.userinfo}
			})
			got, err := UserMembershipExpiry(tc.level)
			if err != nil || got != tc.want {
				t.Fatalf("expiry = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// Free or unknown membership does not trigger a request, and paid membership requires a session.
func TestUserMembershipExpiryRequiresPaidSession(t *testing.T) {
	fakeLibraryRPC(t, func(string, map[string]json.RawMessage) any {
		t.Error("unexpected expiry request")
		return nil
	})
	setSession(Session{})
	for _, level := range []Membership{MembershipFree, MembershipUnknown, "other"} {
		if got, err := UserMembershipExpiry(level); got != "" || err != nil {
			t.Fatalf("expiry for %q = %q, %v", level, got, err)
		}
	}
	if got, err := UserMembershipExpiry(MembershipGreen); got != "" || err == nil {
		t.Fatalf("signed-out expiry = %q, %v", got, err)
	}
}

// Malformed upstream data is reported rather than interpreted as a date.
func TestUserMembershipExpiryMalformedResponse(t *testing.T) {
	fakeLibraryRPC(t, func(string, map[string]json.RawMessage) any {
		return map[string]any{"identity": "invalid"}
	})
	if got, err := UserMembershipExpiry(MembershipGreen); got != "" || err == nil {
		t.Fatalf("malformed expiry = %q, %v", got, err)
	}
}

// Both login channels send their music credentials with the expiry request.
func TestUserMembershipExpiryAuthentication(t *testing.T) {
	for _, loginType := range []int{1, 2} {
		t.Run(string(rune('0'+loginType)), func(t *testing.T) {
			fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Comm map[string]any `json:"comm"`
					Req  struct {
						Module string         `json:"module"`
						Method string         `json:"method"`
						Param  map[string]any `json:"param"`
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Comm["qq"] != "10001" || body.Comm["authst"] != "fake" || body.Comm["tmeLoginType"] != float64(loginType) || body.Req.Module != "VipLogin.VipLoginInter" || body.Req.Method != "vip_login_base" || body.Req.Param == nil || len(body.Req.Param) != 0 {
					t.Error("incorrect authenticated expiry request")
				}
				for _, name := range []string{"uin", "qqmusic_uin", "qm_keyst", "qqmusic_key"} {
					if cookie, err := r.Cookie(name); err != nil || cookie.Value == "" {
						t.Errorf("missing cookie %s", name)
					}
				}
				_, _ = w.Write([]byte(`{"code":0,"req_0":{"code":0,"data":{"identity":{"overdate":"2030-01-02"}}}}`))
			})
			old := httpClient
			httpClient = authHTTP
			t.Cleanup(func() { httpClient = old })
			setSession(Session{MusicID: "10001", MusicKey: "fake", LoginType: loginType})
			if got, err := UserMembershipExpiry(MembershipGreen); got != "2030-01-02" || err != nil {
				t.Fatalf("expiry = %q, %v", got, err)
			}
		})
	}
}
