# Vendored dependency patches

Dependencies are vendored so normal `go test`, `go tool mygo dev` and
`go tool mygo build` include the Windows menu fix. Versions remain those in
`go.mod` / `go.sum`; their licenses are included in `vendor/`.

## MyGo Windows menu width

Base: `github.com/egoist/mygo v0.7.3`.

`mygo-windows-menu-width.patch` fixes Windows 11 owner-drawn popup menus.
Measurement now reports the complete width used to draw labels, padding,
check/icon columns, shortcuts and submenu arrows. It no longer subtracts
`SM_CXMENUCHECK`, which is larger than the width Windows actually adds on
the tested system (31 px versus 26 px at 192 DPI), causing clipped text.

The patch includes a Windows regression test for Chinese/mixed text and
menu columns at 96, 144 and 192 DPI. It does not change application menu
labels, actions or the dependency version.

After intentionally regenerating dependencies, reapply and verify:

```powershell
go mod vendor
git apply patches/mygo-windows-menu-width.patch
go test github.com/egoist/mygo/internal/windows -run TestMenuItemWidthContainsDrawnContent -count=5
go test ./internal/...
go tool mygo vet ./internal
```

Review the patch against the new source before reapplying after a MyGo
upgrade. `-mod=mod` bypasses vendored fixes.
