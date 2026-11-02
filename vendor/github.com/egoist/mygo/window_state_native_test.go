//go:build !js

package mygo

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

type nativeStateTestScreen struct{ platform.Screen }

func (nativeStateTestScreen) Displays() []platform.Display {
	return []platform.Display{{Primary: true, WorkArea: platform.Rect{Width: 1920, Height: 1040}}}
}

type nativeStateTestBackend struct {
	platform.Backend
	restored *platform.Rect
	accept   bool
}

func (b *nativeStateTestBackend) Screen() platform.Screen { return nativeStateTestScreen{} }
func (b *nativeStateTestBackend) RestoreWindowState(o *platform.WindowOptions, r *platform.Rect) bool {
	b.restored = r
	if !b.accept || r == nil {
		return false
	}
	o.StateBounds, o.Center = r, false
	return true
}

type nativeStateTestWindow struct {
	platform.Window
	bounds, pixels                   platform.Rect
	maximized, fullscreen, minimized bool
}

func (w *nativeStateTestWindow) Bounds() platform.Rect            { return w.bounds }
func (w *nativeStateTestWindow) WindowStateBounds() platform.Rect { return w.pixels }
func (w *nativeStateTestWindow) IsMaximized() bool                { return w.maximized }
func (w *nativeStateTestWindow) IsFullScreen() bool               { return w.fullscreen }
func (w *nativeStateTestWindow) IsMinimized() bool                { return w.minimized }

func nativeStateTestSetup(t *testing.T, states map[string]WindowState) *nativeStateTestBackend {
	t.Helper()
	previousBackend, previousStates := backend(), windowStates
	b := &nativeStateTestBackend{Backend: previousBackend, accept: true}
	theBackend = b
	windowStates.loaded, windowStates.byKey, windowStates.dirty = true, states, false
	t.Cleanup(func() { theBackend, windowStates = previousBackend, previousStates })
	return b
}

// Restoring native pixel bounds bypasses ambiguous DIP screen checks and survives a JSON round trip.
func TestNativeWindowStateRestore(t *testing.T) {
	want := Rectangle{X: 4000, Y: 100, Width: 1575, Height: 1050}
	original := WindowState{X: 2286, Y: 57, Width: 900, Height: 600, Maximized: true, WindowsBounds: &want}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var saved WindowState
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	b := nativeStateTestSetup(t, map[string]WindowState{"main": saved})
	opts := &platform.WindowOptions{Width: 1180, Height: 780, Center: true, UseContentSize: true}
	restoreWindowState("main", opts)
	if b.restored == nil || *b.restored != (platform.Rect{X: 4000, Y: 100, Width: 1575, Height: 1050}) {
		t.Fatalf("native bounds not restored: %+v", b.restored)
	}
	if opts.StateBounds == nil || opts.Center || opts.UseContentSize || !opts.Maximized || opts.Width != 900 || opts.Height != 600 {
		t.Fatalf("restored options = %+v", opts)
	}
}

// Legacy or disconnected Windows placements center once without reusing ambiguous DIP coordinates.
func TestNativeWindowStateFallback(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		t.Run(map[bool]string{true: "legacy", false: "disconnected"}[legacy], func(t *testing.T) {
			st := WindowState{X: 100, Y: 100, Width: 2200, Height: 1200, FullScreen: true}
			if !legacy {
				st.WindowsBounds = &Rectangle{X: 8000, Y: 100, Width: 2200, Height: 1200}
			}
			b := nativeStateTestSetup(t, map[string]WindowState{"main": st})
			b.accept = false
			opts := &platform.WindowOptions{}
			restoreWindowState("main", opts)
			if !opts.Center || opts.StateBounds != nil || opts.Width != 1920 || opts.Height != 1040 || !opts.FullScreen {
				t.Fatalf("fallback options = %+v", opts)
			}
		})
	}
}

// Pixel placement is captured without DIP rounding and normal bounds survive maximized, fullscreen and minimized states.
func TestNativeWindowStateCapture(t *testing.T) {
	nativeStateTestSetup(t, map[string]WindowState{})
	n := &nativeStateTestWindow{
		bounds: platform.Rect{X: 2286, Y: 57, Width: 900, Height: 600},
		pixels: platform.Rect{X: 4001, Y: 101, Width: 1575, Height: 1050},
	}
	w := &Window{native: n, stateKey: "main"}
	w.captureState()
	saved := windowStates.byKey["main"]
	if saved.WindowsBounds == nil || *saved.WindowsBounds != (Rectangle{X: 4001, Y: 101, Width: 1575, Height: 1050}) {
		t.Fatalf("captured bounds = %+v", saved.WindowsBounds)
	}
	windowStates.dirty = false
	w.captureState()
	if windowStates.dirty {
		t.Fatal("unchanged bounds dirtied the store")
	}
	n.bounds, n.pixels = platform.Rect{Width: 1920, Height: 1040}, platform.Rect{Width: 3840, Height: 2080}
	for _, state := range []string{"maximized", "fullscreen", "minimized"} {
		n.maximized, n.fullscreen, n.minimized = state == "maximized", state == "fullscreen", state == "minimized"
		w.captureState()
		st := windowStates.byKey["main"]
		if st.WindowsBounds != saved.WindowsBounds || st.X != saved.X || st.Y != saved.Y || st.Width != saved.Width || st.Height != saved.Height {
			t.Fatalf("%s overwrote normal geometry: %+v", state, st)
		}
		if st.Maximized != n.maximized || st.FullScreen != n.fullscreen {
			t.Fatalf("%s flags = %+v", state, st)
		}
	}
}
