package qqmusic

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// QQ comment pages must use the preceding opaque cursor, including IDs too large for int64.
func TestCommentsKeepOpaqueCursor(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("pagenum")
		if page == "0" {
			_, _ = io.WriteString(w, `{"code":0,"comment":{"commenttotal":2,"commentlist":[{"rootcommentid":"18446744073709551615","rootcommentcontent":"one","nick":"listener"}]}}`)
			return
		}
		if r.URL.Query().Get("lasthotcommentid") != "18446744073709551615" {
			t.Error("opaque comment cursor was lost")
		}
		_, _ = io.WriteString(w, `{"code":0,"comment":{"commenttotal":2,"commentlist":[{"rootcommentid":"123","rootcommentcontent":"two"}]}}`)
	})
	first, err := ThreadComments("42", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !first.More || len(first.Latest) != 1 || first.Latest[0].ID == 0 {
		t.Fatalf("first page: %+v", first)
	}
	next, err := ThreadComments("42", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if next.More || len(next.Latest) != 1 || next.Latest[0].Content != "two" {
		t.Fatalf("next page: %+v", next)
	}
}

// Personal radio must keep the QQ track MID so the existing player can resolve it.
func TestPersonalRadioTrackIdentity(t *testing.T) {
	fakeLibraryRPC(t, func(method string, param map[string]json.RawMessage) any {
		if method != "get_radio_track" || string(param["id"]) != "99" {
			t.Errorf("unexpected radio %s: %v", method, param)
		}
		return map[string]any{"tracks": []map[string]any{{"id": 42, "mid": "qqRadioMid", "title": "Radio", "interval": 120}}}
	})
	tracks, err := PersonalFM(FMDefault)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].MID != "qqRadioMid" || tracks[0].Duration != 120000 {
		t.Fatalf("radio tracks: %+v", tracks)
	}
}
