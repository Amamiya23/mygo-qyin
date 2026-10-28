package qqmusic

import (
	"os"
	"testing"
)

// QQ's public login endpoint must return an actual PNG and a waiting QR status.
func TestLiveQQQRCode(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1 to contact QQ Music")
	}
	key, err := QRKey()
	if err != nil {
		t.Fatal(err)
	}
	png, err := QRImage(key)
	if err != nil || len(png) < 8 {
		t.Fatalf("QR image: %d bytes, %v", len(png), err)
	}
	code, _, _, _, err := QRCheck(key)
	if err != nil {
		t.Fatal(err)
	}
	if code != QRWaiting {
		t.Fatalf("new QR status %d", code)
	}
	t.Logf("QQ returned a %d-byte QR image and waiting status", len(png))
}
