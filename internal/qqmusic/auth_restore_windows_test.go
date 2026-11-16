package qqmusic

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/egoist/mygo"
)

// A separate service keeps this platform check away from the listener's real
// credentials. The child process verifies that Credential Manager outlives us.
func TestWindowsSessionSecretAcrossProcesses(t *testing.T) {
	const childService = "LILT_AUTH_TEST_SECRET_SERVICE"
	s := Session{MusicID: "10001", MusicKey: "test-key", LoginType: 2, RefreshToken: "test-token", RefreshKey: "test-refresh", OpenID: "test-open", UnionID: "test-union", Device: &wechatDevice{Qimei: "q16", Qimei36: "q36", UID: "test-uid", SID: "test-sid"}}
	if service := os.Getenv(childService); service != "" {
		b, err := mygo.Secrets.Service(service).Get(qqSessionSecret)
		if err != nil {
			t.Fatal("child process could not read its test credential")
		}
		var restored Session
		if err := json.Unmarshal(b, &restored); err != nil || restored.Device == nil {
			t.Fatal("child process could not decode its test credential")
		}
		device := *restored.Device == *s.Device
		restored.Device, s.Device = nil, nil
		if restored != s || !device {
			t.Fatal("credential store changed session or refresh fields across processes")
		}
		return
	}
	if os.Getenv("LILT_TEST_SYSTEM_SECRETS") != "1" {
		t.Skip("set LILT_TEST_SYSTEM_SECRETS=1 to test the Windows credential store")
	}
	suffix, err := randomHex(8)
	if err != nil {
		t.Fatal(err)
	}
	service := "dev.amamiya.qqmusic.auth-test." + suffix
	store := mygo.Secrets.Service(service)
	t.Cleanup(func() {
		if err := store.Delete(qqSessionSecret); err != nil {
			t.Error("could not delete isolated test credential")
		}
	})
	if _, err := store.Get(qqSessionSecret); !errors.Is(err, mygo.ErrSecretNotFound) {
		t.Fatal("new test service did not report credential absence")
	}
	if err := store.Set(qqSessionSecret, bytes.Repeat([]byte("x"), mygo.MaxSecretSize)); err != nil {
		t.Fatalf("could not store the documented maximum credential size: %v", err)
	}
	b, _ := json.Marshal(s)
	if err := store.Set(qqSessionSecret, b); err != nil {
		t.Fatalf("could not store isolated session: %v", err)
	}
	if err := store.Set(qqSessionSecret, bytes.Repeat([]byte("x"), mygo.MaxSecretSize+1)); err == nil {
		t.Fatal("oversized credential was accepted")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestWindowsSessionSecretAcrossProcesses$")
	cmd.Env = append(os.Environ(), childService+"="+service)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-process credential read failed: %v\n%s", err, output)
	}
}
