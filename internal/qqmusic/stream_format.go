package qqmusic

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/mewkiz/flac/meta"
)

// inspectFLACStream bounds reads even when a CDN ignores Range. Resource and
// format failures allow quality fallback; connection failures remain errors.
func inspectFLACStream(rawURL string) (*meta.StreamInfo, int64, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, ErrUnavailable
	}
	req.Header.Set("Range", "bytes=0-65535")
	req.Header.Set("User-Agent", UserAgent)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, loginFailure("hires-stream", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return nil, 0, ErrUnavailable
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
		return nil, 0, loginHTTPError(res.StatusCode)
	}
	size := int64(0)
	if res.StatusCode == http.StatusOK {
		size = res.ContentLength
	} else if span, total, ok := strings.Cut(res.Header.Get("Content-Range"), "/"); ok && strings.HasPrefix(span, "bytes 0-") {
		size, _ = strconv.ParseInt(total, 10, 64)
	}
	info, err := readFLACInfo(io.LimitReader(res.Body, 65536))
	if err != nil {
		var network net.Error
		if errors.As(err, &network) {
			return nil, 0, loginFailure("hires-stream", err)
		}
		return nil, 0, ErrUnavailable
	}
	return info, max(size, 0), nil
}

func readFLACInfo(input io.Reader) (*meta.StreamInfo, error) {
	r := bufio.NewReader(input)
	h, err := r.Peek(10)
	if err != nil {
		return nil, err
	}
	if string(h[:3]) == "ID3" {
		for _, v := range h[6:10] {
			if v&0x80 != 0 {
				return nil, ErrUnavailable
			}
		}
		size := 10 + (int(h[6])<<21 | int(h[7])<<14 | int(h[8])<<7 | int(h[9]))
		if h[3] == 4 && h[5]&0x10 != 0 {
			size += 10
		}
		if _, err := r.Discard(size); err != nil {
			return nil, err
		}
	}
	var signature [4]byte
	if _, err := io.ReadFull(r, signature[:]); err != nil {
		return nil, err
	}
	if string(signature[:]) != "fLaC" {
		return nil, ErrUnavailable
	}
	block, err := meta.New(r)
	if err != nil {
		return nil, err
	}
	if block.Type != meta.TypeStreamInfo || block.Length != 34 {
		return nil, ErrUnavailable
	}
	if err := block.Parse(); err != nil {
		return nil, err
	}
	return block.Body.(*meta.StreamInfo), nil
}
