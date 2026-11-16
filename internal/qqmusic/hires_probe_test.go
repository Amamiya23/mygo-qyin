package qqmusic

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestHiResProbeHeader(t *testing.T) {
	header := hiResTestHeader()
	for _, tc := range []struct {
		name   string
		body   []byte
		status int
		ok     bool
	}{
		{"FLAC", header, 206, true},
		{"server ignores Range", header, 200, true},
		{"ID3 prefix", append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}, header...), 206, true},
		{"missing resource", header, 404, false},
		{"HTML response", []byte("<html>not audio</html>"), 200, false},
		{"truncated header", header[:30], 206, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := httpClient
			t.Cleanup(func() { httpClient = previous })
			httpClient = &http.Client{Transport: authRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Range") != "bytes=0-65535" {
					t.Error("missing bounded Range request")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(bytes.NewReader(tc.body))}, nil
			})}
			info, _, err := inspectFLACStream("https://dl.stream.qqmusic.qq.com/RS01test.flac?vkey=private")
			if !tc.ok {
				if err == nil {
					t.Fatal("accepted unavailable or invalid audio")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if info.BitsPerSample != 24 || info.SampleRate != 96000 || info.NChannels != 2 {
				t.Fatalf("incorrect audio metadata: %+v", info)
			}
		})
	}
}

func TestHiResProbeErrorRedactsURL(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://example.com/?vkey=private", Err: errors.New("private")}
	if message := hiResProbeError(err); strings.Contains(message, "private") || strings.Contains(message, "example.com") {
		t.Fatal("diagnostics leaked signed URL")
	}
}

func hiResTestHeader() []byte {
	// Minimal STREAMINFO: 24-bit, 96 kHz, stereo. No other metadata or audio
	// frames are needed to identify the format returned by the CDN.
	header := make([]byte, 42)
	copy(header, "fLaC")
	header[4], header[7] = 0x80, 34
	binary.BigEndian.PutUint16(header[8:10], 4096)
	binary.BigEndian.PutUint16(header[10:12], 4096)
	binary.BigEndian.PutUint64(header[18:26], uint64(96000)<<44|uint64(1)<<41|uint64(23)<<36|96000)
	return header
}
