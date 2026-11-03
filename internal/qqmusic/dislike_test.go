package qqmusic

import (
	"encoding/json"
	"testing"
)

func TestAddDislike(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind DislikeType
		key  string
	}{
		{"song", DislikeSong, "Songs"},
		{"singer", DislikeSinger, "Singers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response any = map[string]any{"Retcode": 0}
			calls := 0
			fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
				calls++
				if module != "music.feedback.FeedbackBlack" || method != "AddDislike" || len(param) != 1 {
					t.Errorf("unexpected request: %s/%s %v", module, method, param)
				}
				var items []struct {
					ID     string
					IdType DislikeType
				}
				if err := json.Unmarshal(param[tc.key], &items); err != nil || len(items) != 1 || items[0].ID != "123" || items[0].IdType != tc.kind {
					t.Errorf("incorrect feedback items: %s", param[tc.key])
				}
				return response
			})
			if err := AddDislike(tc.kind, 123); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []any{map[string]any{}, map[string]any{"Retcode": 5}, map[string]any{"Retcode": nil}} {
				response = bad
				if err := AddDislike(tc.kind, 123); err == nil {
					t.Fatalf("accepted response %v", bad)
				}
			}
			before := calls
			if AddDislike(tc.kind, 0) == nil || AddDislike(99, 123) == nil {
				t.Fatal("accepted invalid feedback")
			}
			setSession(Session{})
			if AddDislike(tc.kind, 123) == nil || calls != before {
				t.Fatal("invalid or signed-out request reached server")
			}
		})
	}
}
