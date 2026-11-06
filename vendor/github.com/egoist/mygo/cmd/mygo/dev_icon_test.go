package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// macos.icon used to keep the production icon even when its -dev sibling existed.
func TestDevConfigMacOSIcon(t *testing.T) {
	for _, tt := range []struct {
		name     string
		icon     string
		sibling  string
		want     string
		absolute bool
	}{
		{name: "unset", sibling: "icon/AppIcon-dev.icns"},
		{name: "icns fallback", icon: "icon/AppIcon.icns", want: "icon/AppIcon.icns"},
		{name: "icns development", icon: "icon/AppIcon.icns", sibling: "icon/AppIcon-dev.icns", want: "icon/AppIcon-dev.icns"},
		{name: "different extension", icon: "icon/AppIcon.icns", sibling: "icon/AppIcon-dev.car", want: "icon/AppIcon.icns"},
		{name: "asset catalog", icon: "icon/Assets.car", sibling: "icon/Assets-dev.car", want: "icon/Assets-dev.car"},
		{name: "icon composer", icon: "icon/AppIcon.icon", sibling: "icon/AppIcon-dev.icon", want: "icon/AppIcon-dev.icon"},
		{name: "uppercase extension", icon: "icon/AppIcon.ICNS", sibling: "icon/AppIcon-dev.ICNS", want: "icon/AppIcon-dev.ICNS"},
		{name: "absolute path", icon: "icon/AppIcon.icns", sibling: "icon/AppIcon-dev.icns", want: "icon/AppIcon-dev.icns", absolute: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			icon, want := tt.icon, tt.want
			if tt.absolute {
				icon, want = filepath.Join(root, icon), filepath.Join(root, want)
			}
			for _, file := range []string{"assets/icon.png", "assets/icon-dev.png", tt.icon, tt.sibling} {
				if file == "" {
					continue
				}
				if filepath.Ext(file) == ".icon" {
					file = filepath.Join(file, "nested", "icon.json")
				}
				writeDevIconFixture(t, filepath.Join(root, file), []byte("icon"))
			}
			config := Config{
				Name: "Example", Identifier: "com.example.app", Slug: "example",
				Icon: "assets/icon.png", MacOS: MacOS{Icon: icon},
			}
			source, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			configFile := filepath.Join(root, jsonConfig)
			writeDevIconFixture(t, configFile, source)
			c, err := loadConfig(root)
			if err != nil {
				t.Fatal(err)
			}
			before := *c
			d := devConfig(c)
			if d.MacOS.Icon != want {
				t.Errorf("development macos.icon = %q, want %q", d.MacOS.Icon, want)
			}
			if d.Icon != "assets/icon-dev.png" {
				t.Errorf("development icon = %q, want assets/icon-dev.png", d.Icon)
			}
			if !reflect.DeepEqual(*c, before) {
				t.Error("devConfig modified the source configuration")
			}
			if d == c {
				t.Error("devConfig returned the source configuration")
			}
			data, err := os.ReadFile(configFile)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, source) {
				t.Error("devConfig modified the source configuration file")
			}
		})
	}
}

// Development bundles load the macOS ICNS override and invalidate it when its bytes change.
func TestDevSessionMacOSICNS(t *testing.T) {
	root := t.TempDir()
	c := &Config{root: root, Icon: "missing.png", MacOS: MacOS{Icon: "icon/AppIcon.icns"}}
	production := []byte("icns production fixture")
	development := []byte("icns development fixture")
	writeDevIconFixture(t, c.path(c.MacOS.Icon), production)
	devPath := c.path("icon/AppIcon-dev.icns")
	writeDevIconFixture(t, devPath, development)
	d := devConfig(c)
	var session devSession
	// devSession.icon previously ignored macos.icon and rendered the full-canvas PNG.
	got, err := session.icon(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, development) {
		t.Fatal("development bundle did not use its macOS ICNS")
	}
	updated := []byte("icns updated development fixture")
	writeDevIconFixture(t, devPath, updated)
	got, err = session.icon(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, updated) {
		t.Fatal("development ICNS cache did not invalidate")
	}
	got, err = session.icon(c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, production) {
		t.Fatal("switching back to production reused the development ICNS")
	}
	// With no override, development bundles must still render their shared PNG.
	c.MacOS.Icon = ""
	c.Icon = "icon.png"
	writeDevIconFixture(t, c.path(c.Icon), defaultIcon())
	want, err := appIcon(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err = session.icon(c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("PNG fallback no longer renders correctly")
	}
}

// The watcher used to miss macOS icons outside resources, including a newly created -dev sibling.
func TestBuildInputsMacOSIcon(t *testing.T) {
	for _, ext := range []string{".icns", ".car", ".icon"} {
		t.Run(ext, func(t *testing.T) {
			root := t.TempDir()
			writeDevIconFixture(t, filepath.Join(root, "go.mod"), []byte("module example.com/devicon\n\ngo 1.25.0\n"))
			writeDevIconFixture(t, filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"))
			c := &Config{root: root, Main: ".", MacOS: MacOS{Icon: "icon/AppIcon" + ext}}
			in, err := listBuildInputs(c)
			if err != nil {
				t.Fatal(err)
			}
			icons := []string{c.path(c.MacOS.Icon), c.path("icon/AppIcon-dev" + ext)}
			before := snapshot(in)
			for _, icon := range icons {
				if stamp, ok := before[icon]; !ok || stamp != "-" {
					t.Errorf("missing icon %q is not watched: %q, present=%v", icon, stamp, ok)
				}
			}
			for _, icon := range icons {
				file := icon
				if ext == ".icon" {
					file = filepath.Join(icon, "nested", "icon.json")
				}
				writeDevIconFixture(t, file, []byte("original"))
				created := snapshot(in)
				if stamp, ok := created[file]; !ok || stamp == "-" {
					t.Errorf("created icon content %q is not watched", file)
				}
				writeDevIconFixture(t, file, []byte("changed icon content"))
				changed := snapshot(in)
				if changed[file] == created[file] {
					t.Errorf("editing %q did not change its snapshot", file)
				}
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if removed := snapshot(in); removed[file] == changed[file] {
					t.Errorf("removing %q did not change its snapshot", file)
				}
			}
		})
	}
}

func writeDevIconFixture(t *testing.T, file string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
