package qqmusic

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAlbumCompanyPreservesRightsNotice(t *testing.T) {
	const notice = "(P) 2020(5,7),2021(2-4,11) Sony Music Labels Inc. /\n2019(9,13) Sony Music Entertainment (Japan) Inc."
	for _, tc := range []struct {
		name, primary, alternate, want string
	}{
		{"primary notice", notice, "未确定", notice},
		{"empty primary", "", notice, notice},
		{"blank primary", "  ", notice, notice},
		{"placeholder primary", "未确定", notice, notice},
		{"no company", "", "", ""},
		{"both placeholders", "未确定", "未确定", ""},
		{"alternate placeholder", "", "未确定", ""},
		{"padded placeholder", " \n未确定\t", "  ", ""},
		{"traditional placeholder", "未確定", "未確定", ""},
		{"independent release", "独立发行", "", "独立发行"},
		{"only exact placeholders", "未确定音乐工作室", "", "未确定音乐工作室"},
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
