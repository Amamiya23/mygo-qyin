package qqmusic

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAlbumDetailCompany(t *testing.T) {
	const notice = "(P) 2020(5,7),2021(2-4,11) Sony Music Labels Inc. /\n2019(9,13) Sony Music Entertainment (Japan) Inc."
	for _, tc := range []struct {
		name, primary, alternate, want string
	}{
		{"notice", notice, "索尼音乐", notice},
		{"fallback", " \n未确定\t", notice, notice},
		{"missing", "", "未確定", ""},
		{"independent", "独立发行", "", "独立发行"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v8/fcg-bin/fcg_v8_album_info_cp.fcg" {
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
				json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
					"id": 24649832, "company": tc.primary, "company_new": map[string]any{"name": tc.alternate},
				}})
			})
			album, err := AlbumDetail(24649832)
			if err != nil {
				t.Fatal(err)
			}
			if album.Company != tc.want {
				t.Fatalf("company = %q; want %q", album.Company, tc.want)
			}
		})
	}
}
