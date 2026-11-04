# Windows taskbar lyrics T0 probe

This independent diagnostic uses the pinned MyGo native renderer. It does not
enable taskbar lyrics in Lilt, read player data, inject into Explorer, or resize
Explorer's task list. Its colors are Radix Slate dark tokens.

Build from the repository root with PowerShell:

~~~powershell
$env:CGO_ENABLED = '0'
go build -o build/taskbar-lyrics-probe.exe ./cmd/taskbar-lyrics-probe
~~~

Run explicitly to show the short probe:

~~~powershell
./build/taskbar-lyrics-probe.exe --run --duration 3s --report build/taskbar-lyrics-probe/report.json --capture-dir build/taskbar-lyrics-probe
~~~

The default --parent auto reads the real OS build with RtlGetVersion. Builds
22000 and newer attach directly to Shell_TrayWnd, while classic Windows 10 uses
ReBarWindow32 or WorkerW. Merely finding WorkerW is not sufficient on a modern
taskbar. Use --parent taskbar or --parent container for an explicit comparison.
The report includes OS build, every sampled window class, and the embedding
host's immediate children in top-to-bottom order. HWND_TOP is the numeric value
zero already passed to SetWindowPos; it is not a different z-order flag.

The lyrics view explicitly clears c.Root().Background(ui.Transparent), because
WindowOptions.Transparent alone does not clear MyGo's default UI theme root.
It also removes DWM's Panel outline/rounded full-window clip using the per-window
border color and corner preference attributes; it does not change MyGo globally.

The four phases last three seconds each: a transparent floating lyrics window,
the same HWND embedded in the primary taskbar, the original HWND detached back
to floating, then a fresh floating window. The ordinary main window redraws its
phase in parallel, so captures can reveal damage to its DPI or render surface.
The program then destroys its own lyrics window and exits. Closing the ordinary
window or using its stop button also restores parenting and cleans up.

The temporary fixed taskbar region starts 12 physical pixels from the left and
is at most 280 DIP wide. It can briefly cover taskbar content, while all pointer
input passes through. The executable without --run only prints usage. No taskbar
free-space algorithm, Explorer restart, automatic taskbar hiding, or production
window integration is included.

The JSON report records per-phase window handles, native parent/owner, style,
physical bounds, DPI awareness, visibility, view build count, foreground window,
and optional screen captures cropped to the test window rectangles. Inspect all
four lyrics images and the main images: a successful SetParent or UI build does
not prove the text was composited. A completed report only means the sequence
ran and its own windows were cleaned up; it is not full platform acceptance.

The report also includes the UI thread's focused HWND, explicit no-activate and
click-through style flags, and the result of asking the lyrics window procedure
about WM_MOUSEACTIVATE. Values 1/2 mean activate, 3/4 mean do not activate. This
query does not simulate a real click. The floating phase intentionally retains
MyGo Panel's default activation style; only the embedded phase adds no-activate.
Screen captures contain final composited RGB pixels, so their alpha channel
cannot establish transparent-surface correctness by itself.

Additional manual cases remain: mixed-DPI displays, taskbar auto-hide, Win+D,
fullscreen apps, Explorer restart and monitor removal. The probe refuses primary
taskbars on the top/side or currently hidden off the bottom of the monitor.
