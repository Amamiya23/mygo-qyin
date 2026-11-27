package qqmusic

import (
	"bufio"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/mewkiz/flac/meta"
)

// streamProbeError redacts signed stream URLs without disguising stream errors as login failures.
type streamProbeError struct {
	err error
}

func (e *streamProbeError) Unwrap() error { return e.err }

func (e *streamProbeError) Error() string {
	var dns *net.DNSError
	if errors.As(e.err, &dns) {
		return "音频流服务无法连接，请检查网络后重试"
	}
	var certificate x509.UnknownAuthorityError
	var hostname x509.HostnameError
	if errors.As(e.err, &certificate) || errors.As(e.err, &hostname) {
		return "音频流证书校验失败，请检查网络后重试"
	}
	var network net.Error
	if errors.As(e.err, &network) {
		if network.Timeout() {
			return "音频流连接超时，请检查网络后重试"
		}
		if !errors.Is(e.err, io.EOF) && !errors.Is(e.err, io.ErrUnexpectedEOF) {
			return "音频流连接失败，请检查网络后重试"
		}
	}
	if errors.Is(e.err, io.EOF) || errors.Is(e.err, io.ErrUnexpectedEOF) {
		return "音频流连接被中断，请重试"
	}
	return "音频流探测失败，请检查网络后重试"
}

// inspectFLACStream bounds reads even when a CDN ignores Range. Resource and
// format failures allow quality fallback; connection failures remain errors.
func inspectFLACStream(ctx context.Context, rawURL string) (*meta.StreamInfo, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, ErrUnavailable
	}
	req.Header.Set("Range", "bytes=0-65535")
	req.Header.Set("User-Agent", UserAgent)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, &streamProbeError{err: err}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
		return nil, 0, ErrUnavailable
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
			return nil, 0, &streamProbeError{err: err}
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
