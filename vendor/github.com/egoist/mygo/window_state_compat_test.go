package mygo

import (
	"encoding/json"
	"testing"
)

// The upstream pixel fields must retain the saved physical placement from Lilt's old windowsBounds format.
func TestWindowStateLegacyPixels(t *testing.T) {
	for _, pixels := range []string{"", `,"pixelX":4001,"pixelY":101,"pixelWidth":1575,"pixelHeight":1050`} {
		data := `{"x":2286,"y":57,"width":900,"height":600,"maximized":true,"windowsBounds":{"x":4000,"y":100,"width":1575,"height":1050}` + pixels + `}`
		var st WindowState
		if err := json.Unmarshal([]byte(data), &st); err != nil {
			t.Fatal(err)
		}
		wantX, wantY := 4000, 100
		if pixels != "" {
			wantX, wantY = 4001, 101
		}
		if st.PixelX != wantX || st.PixelY != wantY || st.PixelWidth != 1575 || st.PixelHeight != 1050 || st.Width != 900 || st.Height != 600 || !st.Maximized {
			t.Fatalf("decoded state = %+v", st)
		}
		saved, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(saved, &fields); err != nil {
			t.Fatal(err)
		}
		if _, legacy := fields["windowsBounds"]; legacy {
			t.Fatalf("saved legacy format: %s", saved)
		}
		var restored WindowState
		if err := json.Unmarshal(saved, &restored); err != nil {
			t.Fatal(err)
		}
		if restored != st {
			t.Fatalf("round trip changed placement: %+v", restored)
		}
	}
}
