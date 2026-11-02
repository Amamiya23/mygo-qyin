// Package control lets other programs drive the running app: the app serves
// a Unix socket, which the CLI (lilt ctl) and the MCP server (lilt mcp) call.
package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

// Param is one argument of a command. Type is a JSON Schema type, or
// "integers" for an array of integers.
type Param struct {
	Name     string
	Type     string
	Help     string
	Required bool
	Enum     []string
}

type Command struct {
	Name   string
	Help   string
	Params []Param
}

var (
	songIDs = Param{Name: "song_ids", Type: "integers", Help: "song IDs (numeric `id` from search/playlist results)"}
	plID    = Param{Name: "id", Type: "integer", Help: "playlist ID", Required: true}
	offset  = Param{Name: "offset", Type: "integer", Help: "items to skip (default 0)"}
	limit   = Param{Name: "limit", Type: "integer", Help: "items to return (default 50)"}
)

func required(p Param) Param { p.Required = true; return p }

// Commands are what the app answers; the CLI and the MCP tools follow them.
var Commands = []Command{
	{Name: "status", Help: "Current track, playback state, position, volume, shuffle/repeat and signed-in user."},
	{Name: "account", Help: "The signed-in QQ Music account."},
	{Name: "search", Help: "Search QQ Music for songs, albums or artists.", Params: []Param{
		{Name: "query", Type: "string", Help: "search keywords", Required: true},
		{Name: "type", Type: "string", Help: "what to search (default song)", Enum: []string{"song", "album", "artist"}},
		offset,
		{Name: "limit", Type: "integer", Help: "results to return (default 20, max 50)"},
	}},
	{Name: "playlists", Help: "The signed-in user's playlists: liked songs, created and saved playlists."},
	{Name: "playlist", Help: "A playlist's details and songs.", Params: []Param{plID, offset, limit}},
	{Name: "album", Help: "An album's details and songs.", Params: []Param{{Name: "id", Type: "integer", Help: "album ID", Required: true}}},
	{Name: "artist", Help: "An artist's profile and hot songs.", Params: []Param{{Name: "id", Type: "integer", Help: "artist ID", Required: true}}},
	{Name: "play", Help: "Replace the queue and play: give exactly one of song_ids, playlist_id or album_id.", Params: []Param{
		songIDs,
		{Name: "playlist_id", Type: "integer", Help: "play this playlist"},
		{Name: "album_id", Type: "integer", Help: "play this album"},
		{Name: "index", Type: "integer", Help: "index of the song to start with (default the first playable)"},
	}},
	{Name: "pause", Help: "Pause playback."},
	{Name: "resume", Help: "Resume playback."},
	{Name: "next", Help: "Play the next song in the queue."},
	{Name: "prev", Help: "Play the previous song in the queue."},
	{Name: "seek", Help: "Seek within the current song.", Params: []Param{{Name: "seconds", Type: "number", Help: "position in seconds", Required: true}}},
	{Name: "volume", Help: "Set the volume.", Params: []Param{{Name: "level", Type: "integer", Help: "0–100", Required: true}}},
	{Name: "shuffle", Help: "Turn shuffle on or off.", Params: []Param{{Name: "on", Type: "boolean", Help: "shuffle on", Required: true}}},
	{Name: "repeat", Help: "Set the repeat mode.", Params: []Param{{Name: "mode", Type: "string", Required: true, Enum: []string{"off", "all", "one"}}}},
	{Name: "queue", Help: "The play queue and the current song's index in it.", Params: []Param{offset, limit}},
	{Name: "jump", Help: "Play the queue's song at index.", Params: []Param{{Name: "index", Type: "integer", Help: "queue index", Required: true}}},
	{Name: "like", Help: "Like a song (adds it to 我喜欢) or unlike it.", Params: []Param{
		{Name: "song_id", Type: "integer", Help: "song ID", Required: true},
		{Name: "liked", Type: "boolean", Help: "false to unlike (default true)"},
	}},
	{Name: "playlist_create", Help: "Create a playlist, optionally with songs.", Params: []Param{
		{Name: "name", Type: "string", Help: "1–20 characters", Required: true},
		songIDs,
	}},
	{Name: "playlist_rename", Help: "Rename a playlist the user created.", Params: []Param{plID, {Name: "name", Type: "string", Help: "1–20 characters", Required: true}}},
	{Name: "playlist_delete", Help: "Delete a playlist the user created, or unsave a saved one. Cannot be undone.", Params: []Param{plID}},
	{Name: "playlist_add", Help: "Add songs to a playlist the user created (at most 100 at once).", Params: []Param{plID, required(songIDs)}},
	{Name: "playlist_remove", Help: "Remove songs from a playlist the user created.", Params: []Param{plID, required(songIDs)}},
}

func lookup(name string) *Command {
	for i := range Commands {
		if Commands[i].Name == name {
			return &Commands[i]
		}
	}
	return nil
}

type request struct {
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

type response struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// Handler answers a command with a value to encode as JSON.
type Handler func(cmd string, args json.RawMessage) (any, error)

// SocketPath is where the app listens: $LILT_SOCKET, or control.sock in the
// app's user data directory, apart for the development app.
func SocketPath() (string, error) {
	if p := os.Getenv("LILT_SOCKET"); p != "" {
		return p, nil
	}
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "control.sock"), nil
}

// Serve listens on the socket until the returned stop is called. Another
// instance already listening keeps the socket.
func Serve(h Handler) (stop func(), err error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Close()
		return nil, fmt.Errorf("control socket %s is in use", path)
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// Only this user may drive the app.
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Printf("control: %v", err)
				}
				return
			}
			go serveConn(c, h)
		}
	}()
	return func() { ln.Close(); os.Remove(path) }, nil
}

func serveConn(c net.Conn, h Handler) {
	defer c.Close()
	var req request
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
		return
	}
	var res response
	if lookup(req.Cmd) == nil {
		res.Error = "unknown command " + req.Cmd
	} else if data, err := h(req.Cmd, req.Args); err != nil {
		res.Error = err.Error()
	} else {
		res.OK, res.Data = true, data
	}
	_ = json.NewEncoder(c).Encode(res)
}

// ErrNotRunning says no app answers on the socket.
var ErrNotRunning = errors.New("Lilt is not running: start the app first")

// Call sends cmd to the running app and returns its answer as JSON.
func Call(ctx context.Context, cmd string, args json.RawMessage) (json.RawMessage, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, ErrNotRunning
	}
	defer c.Close()
	// Searches and playlist writes wait on QQ Music.
	deadline := time.Now().Add(90 * time.Second)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	c.SetDeadline(deadline)
	if err := json.NewEncoder(c).Encode(request{Cmd: cmd, Args: args}); err != nil {
		return nil, err
	}
	var res struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.NewDecoder(c).Decode(&res); err != nil {
		return nil, fmt.Errorf("reading the app's answer: %w", err)
	}
	if !res.OK {
		return nil, errors.New(res.Error)
	}
	if res.Data == nil {
		res.Data = json.RawMessage("{}")
	}
	return res.Data, nil
}
