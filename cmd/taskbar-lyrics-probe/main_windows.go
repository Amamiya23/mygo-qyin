//go:build windows

// taskbar-lyrics-probe exercises MyGo's native renderer across Explorer parenting.
// It is a diagnostic program, not a supported taskbar-lyrics mode.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type sample struct {
	Phase          string           `json:"phase"`
	At             time.Time        `json:"at"`
	Lyrics         windowSnapshot   `json:"lyrics"`
	Main           windowSnapshot   `json:"main"`
	Foreground     string           `json:"foreground"`
	ThreadFocus    string           `json:"threadFocus"`
	MouseActivate  uint32           `json:"syntheticMouseActivateResult"`
	HostChildren   []windowSnapshot `json:"hostChildrenTopToBottom,omitempty"`
	LyricsBuilds   int              `json:"lyricsBuilds"`
	Screenshot     string           `json:"screenshot,omitempty"`
	MainScreenshot string           `json:"mainScreenshot,omitempty"`
	Error          string           `json:"error,omitempty"`
}

type report struct {
	Started           time.Time      `json:"started"`
	Completed         bool           `json:"completed"`
	Taskbar           windowSnapshot `json:"taskbar"`
	OS                osVersion      `json:"os"`
	ParentMode        string         `json:"parentMode"`
	InitialForeground string         `json:"initialForeground"`
	Samples           []sample       `json:"samples"`
	Cleanup           bool           `json:"cleanup"`
	Error             string         `json:"error,omitempty"`
	Limitations       []string       `json:"limitations"`
}

type probe struct {
	main, lyrics           *mygo.Window
	phase                  string
	builds                 int
	anchor                 attachment
	duration               time.Duration
	reportPath, captureDir string
	parentMode             string
	result                 report
	stopped                bool
}

func main() {
	run := flag.Bool("run", false, "run the short test; temporarily draws in a fixed taskbar region")
	parentMode := flag.String("parent", "auto", "Explorer parent: auto (OS build policy), taskbar, or container")
	duration := flag.Duration("duration", 3*time.Second, "time spent in each of four phases (minimum 1s)")
	reportPath := flag.String("report", "build/taskbar-lyrics-probe/report.json", "JSON diagnostics destination")
	captureDir := flag.String("capture-dir", "", "optional PNG captures of the lyrics window's screen rectangle")
	flag.Parse()
	if !*run {
		flag.Usage()
		return
	}
	if *parentMode != "auto" && *parentMode != "taskbar" && *parentMode != "container" {
		log.Fatal("parent must be auto, taskbar or container")
	}
	if *duration < time.Second {
		log.Fatal("duration must be at least 1s")
	}
	p := &probe{duration: *duration, reportPath: *reportPath, captureDir: *captureDir, parentMode: *parentMode}
	p.result = report{Started: time.Now(), InitialForeground: handleString(foreground()), Limitations: []string{
		"Fixed 280 DIP taskbar region; no automatic free-space detection or task-list resizing.",
		"Primary bottom taskbar only; the program refuses side/top taskbars.",
		"Screenshots and native parent/style checks do not establish all DPI, auto-hide, Explorer-restart or fullscreen behavior.",
	}}
	p.result.OS = operatingSystem()
	p.result.ParentMode = *parentMode
	mygo.App.SetName("Lilt Taskbar Lyrics Probe")
	mygo.App.WhenReady(p.start)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { p.finish(false) })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if p.result.Error != "" {
		log.Print(p.result.Error)
		os.Exit(1)
	}
}

func (p *probe) start() {
	p.phase = "floating"
	p.main = mygo.NewWindow(mygo.WindowOptions{Title: "Lilt · taskbar T0 probe", Width: 460, Height: 180, Hidden: true, Content: ui.View(p.mainView)})
	p.main.OnClosed(func() { p.finish(false); mygo.App.Quit() })
	p.main.ShowInactive()
	p.newLyrics()
	p.phase = "floating"
	p.lyrics.ShowInactive()
	p.schedule(p.duration/2, p.capture)
	p.schedule(p.duration, p.embed)
}

func (p *probe) mainView(c *ui.Context) {
	// Radix Slate dark step 1 background and step 12 text.
	ui.Column(c).Fill().Padding(24).Gap(12).Background(ui.Hex("#111113")).Children(func() {
		ui.Text(c, "任务栏歌词 · T0").FontSize(22).TextColor(ui.Hex("#edeef0"))
		ui.Text(c, p.phase).FontSize(14).TextColor(ui.Hex("#b0b4ba"))
		ui.Text(c, "浮窗 → 任务栏 → 返回浮窗 → 重建窗口").FontSize(13).TextColor(ui.Hex("#b0b4ba"))
		ui.Button(c, "结束验证").OnClick(func() { p.finish(false); mygo.App.Quit() })
	})
}

func (p *probe) newLyrics() {
	p.lyrics = mygo.NewWindow(mygo.WindowOptions{Title: "Lilt · taskbar T0 lyrics", Panel: true, Hidden: true, Frameless: true, Transparent: true, DisableShadow: true, DisableResize: true, Width: 280, Height: 42, Content: ui.View(p.lyricsView)})
	p.lyrics.SetIgnoreMouseEvents(true)
	clearPanelFrame(p.lyrics.NativeHandle())
	placeFloating(p.lyrics.NativeHandle())
}

func (p *probe) lyricsView(c *ui.Context) {
	p.builds++
	c.Root().Background(ui.Transparent)
	// The background stays transparent; text colors are Radix Slate dark steps 12/11.
	ui.Column(c).Fill().Center().Children(func() {
		ui.Text(c, "晴天 · 君をのせて").FontSize(16).FontWeight(600).TextColor(ui.Hex("#edeef0")).SingleLine()
		ui.Text(c, p.phase).FontSize(10).TextColor(ui.Hex("#b0b4ba")).SingleLine()
	})
}

func (p *probe) schedule(delay time.Duration, fn func()) {
	time.AfterFunc(delay, func() {
		mygo.RunOnMain(func() {
			if !p.stopped {
				fn()
			}
		})
	})
}

func (p *probe) changePhase(phase string) {
	p.phase = phase
	p.main.Invalidate()
	p.lyrics.Invalidate()
	p.schedule(p.duration/2, p.capture)
}

func (p *probe) embed() {
	taskbar, err := primaryBottomTaskbar()
	if err != nil {
		p.fail(err)
		return
	}
	p.result.Taskbar = snapshot(taskbar)
	if err := p.anchor.attach(p.lyrics.NativeHandle(), taskbar, p.parentMode); err != nil {
		p.fail(err)
		return
	}
	p.changePhase("embedded")
	p.schedule(p.duration, p.detach)
}

func (p *probe) detach() {
	if err := p.anchor.detach(); err != nil {
		p.fail(err)
		return
	}
	placeFloating(p.lyrics.NativeHandle())
	p.changePhase("detached")
	p.schedule(p.duration, p.recreate)
}

func (p *probe) recreate() {
	p.lyrics.Destroy()
	p.newLyrics()
	p.lyrics.ShowInactive()
	p.changePhase("recreated")
	p.schedule(p.duration, func() { p.finish(true); mygo.App.Quit() })
}

func (p *probe) capture() {
	s := sample{Phase: p.phase, At: time.Now(), Lyrics: snapshot(p.lyrics.NativeHandle()), Main: snapshot(p.main.NativeHandle()), Foreground: handleString(foreground()), ThreadFocus: handleString(threadFocus()), MouseActivate: mouseActivate(p.lyrics.NativeHandle()), LyricsBuilds: p.builds}
	if p.anchor.attached {
		s.HostChildren = childSnapshots(p.anchor.parent)
	}
	if p.captureDir != "" {
		path := filepath.Join(p.captureDir, p.phase+".png")
		if err := captureWindowRect(p.lyrics.NativeHandle(), path); err != nil {
			s.Error = err.Error()
		} else {
			s.Screenshot = path
		}
		mainPath := filepath.Join(p.captureDir, "main-"+p.phase+".png")
		if err := captureWindowRect(p.main.NativeHandle(), mainPath); err != nil {
			s.Error = err.Error()
		} else {
			s.MainScreenshot = mainPath
		}
	}
	p.result.Samples = append(p.result.Samples, s)
	p.writeReport()
}

func (p *probe) fail(err error) { p.result.Error = err.Error(); p.finish(false); mygo.App.Quit() }

func (p *probe) finish(completed bool) {
	if p.stopped {
		return
	}
	p.stopped = true
	if err := p.anchor.detach(); err != nil {
		p.result.Error = err.Error()
	}
	if p.lyrics != nil && !p.lyrics.IsDestroyed() {
		p.lyrics.Destroy()
	}
	p.result.Cleanup = p.lyrics == nil || p.lyrics.IsDestroyed()
	p.result.Completed = completed && p.result.Error == ""
	p.writeReport()
}

func (p *probe) writeReport() {
	b, err := json.MarshalIndent(p.result, "", "  ")
	if err != nil {
		log.Print(err)
		return
	}
	if err = os.MkdirAll(filepath.Dir(p.reportPath), 0755); err == nil {
		err = os.WriteFile(p.reportPath, append(b, '\n'), 0644)
	}
	if err != nil {
		log.Printf("report: %v", err)
	} else {
		fmt.Printf("%s: %s\n", p.phase, p.reportPath)
	}
}
