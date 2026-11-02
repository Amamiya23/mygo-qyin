package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		cmd  string
		args []string
		want map[string]any
		err  string
	}{
		{cmd: "search", args: []string{"周杰伦", "--type", "album", "--limit=5"}, want: map[string]any{"query": "周杰伦", "type": "album", "limit": int64(5)}},
		{cmd: "playlist_add", args: []string{"42", "1,2, 3"}, want: map[string]any{"id": int64(42), "song_ids": []int64{1, 2, 3}}},
		{cmd: "playlist_add", args: []string{"--song-ids", "7", "42"}, want: map[string]any{"id": int64(42), "song_ids": []int64{7}}},
		{cmd: "like", args: []string{"9", "--liked", "false"}, want: map[string]any{"song_id": int64(9), "liked": false}},
		{cmd: "like", args: []string{"9", "--liked"}, want: map[string]any{"song_id": int64(9), "liked": true}},
		{cmd: "shuffle", args: []string{"true"}, want: map[string]any{"on": true}},
		{cmd: "search", args: []string{"--limit", "3", "--", "--help"}, want: map[string]any{"query": "--help", "limit": int64(3)}},
		{cmd: "repeat", args: []string{"sometimes"}, err: "mode must be one of"},
		{cmd: "playlist_rename", args: []string{"42"}, err: "missing name"},
		{cmd: "status", args: []string{"extra"}, err: "unexpected argument"},
		{cmd: "search", args: []string{"x", "--nope", "1"}, err: "unknown flag"},
	}
	for _, c := range cases {
		got, err := parseArgs(lookup(c.cmd), c.args)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s %v: err = %v, want %q", c.cmd, c.args, err, c.err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %v = %v, %v; want %v", c.cmd, c.args, got, err, c.want)
		}
	}
}

func TestSchemas(t *testing.T) {
	for _, c := range Commands {
		s := schema(c)
		if s.Type != "object" {
			t.Fatalf("%s: type %q", c.Name, s.Type)
		}
		if _, err := s.Resolve(nil); err != nil {
			t.Errorf("%s: %v", c.Name, err)
		}
	}
	if got := schema(*lookup("playlist_add")).Properties["song_ids"]; got.Type != "array" || got.Items.Type != "integer" {
		t.Errorf("song_ids schema = %+v", got)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Setenv("LILT_SOCKET", filepath.Join(t.TempDir(), "c.sock"))
	stop, err := Serve(func(cmd string, args json.RawMessage) (any, error) {
		if cmd == "pause" {
			return nil, errors.New("nothing is playing")
		}
		return map[string]any{"cmd": cmd, "args": args}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if _, err := Serve(nil); err == nil {
		t.Error("a second server took the socket in use")
	}
	data, err := Call(context.Background(), "search", json.RawMessage(`{"query":"x"}`))
	if err != nil || !strings.Contains(string(data), `"query":"x"`) {
		t.Errorf("search = %s, %v", data, err)
	}
	if _, err := Call(context.Background(), "pause", nil); err == nil || err.Error() != "nothing is playing" {
		t.Errorf("pause err = %v", err)
	}
	if _, err := Call(context.Background(), "rm -rf", nil); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("unknown command err = %v", err)
	}
	var out, errOut bytes.Buffer
	if code := cli([]string{"volume", "30"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), `"level": 30`) {
		t.Errorf("cli volume = %d, %q, %q", code, out.String(), errOut.String())
	}
	stop()
	if _, err := Call(context.Background(), "status", nil); !errors.Is(err, ErrNotRunning) {
		t.Errorf("after stop err = %v", err)
	}
}
