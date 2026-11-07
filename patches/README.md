# MyGo 本地补丁

当前依赖为 [MyGo v0.8.1](https://github.com/egoist/mygo/releases/tag/v0.8.1)。
以下补丁均已应用到受 Git 跟踪的 `vendor/github.com/egoist/mygo`，
路径相对 MyGo 根目录，并以干净的 v0.8.1 为基准生成。

| 补丁 | 用途 |
| --- | --- |
| `mygo-v0.8.1-radix-color-swatches.patch` | ColorPicker / ColorWell 内置色块使用 Radix Colors 第 9 阶与黑色端点，保留自选色及透明度。 |
| `mygo-v0.8.1-linux-layer-drag.patch` | Wayland layer-shell 歌词面板通过更新锚点拖动，限制在当前输出内，并通知位置保存。 |
| `mygo-v0.8.1-macos-dev-icon.patch` | 开发包优先选择 `macos.icon` 的 `-dev` 兄弟文件，正确读取和缓存 ICNS，并监听文件或资产目录变化。 |
| `mygo-v0.8.1-windows-state-compat.patch` | 将旧版本地补丁保存的 `windowsBounds` 读入上游 `pixel*` 字段；已有 `pixel*` 优先，下次保存使用上游格式。 |
| `mygo-v0.8.1-tester-label-target.patch` | Tester 点击同名目标时逐个验证实际命中，避免 RichText 父文本误挡内联链接；仍拒绝被遮挡的目标。 |

重新生成 vendor 后，在项目根目录应用全部补丁：

```powershell
go mod vendor
Get-ChildItem patches/mygo-v0.8.1-*.patch | ForEach-Object {
    git apply --check --directory=vendor/github.com/egoist/mygo $_.FullName
    if ($LASTEXITCODE -ne 0) { throw "补丁检查失败：$($_.Name)" }
    git apply --directory=vendor/github.com/egoist/mygo $_.FullName
    if ($LASTEXITCODE -ne 0) { throw "补丁应用失败：$($_.Name)" }
}
```

已应用时，将 `git apply --check` 改为 `git apply --reverse --check` 验证。
升级依赖后需先检查上游是否已包含对应修复，再重新生成或移除补丁。

## Windows 混合 DPI 窗口恢复

v0.8.1 已包含上游原生像素坐标恢复，包括在目标屏幕创建 HWND，避免
`WM_DPICHANGED` 再次缩放已换算尺寸。因此移除原 v0.7.4 Windows 平台算法补丁，
仅保留旧 `windowsBounds` 数据的读取兼容，避免升级后丢失保存的位置。
`scripts/check-window-state` 已使用上游 `PixelX/Y/Width/Height`。

原生检查使用内存状态库与隐藏窗口，不访问用户窗口配置、不修改系统缩放。
需要至少两块不同 DPI 的显示器；缺少条件时默认跳过并退出 2，
可用 `-require-mixed=false` 接受跳过。

```powershell
$env:CGO_ENABLED = "0"
go test ./...
go tool mygo vet .
go test github.com/egoist/mygo -run TestWindowStateLegacyPixels
go test github.com/egoist/mygo/ui -run TestTesterClicksInlineLinkWithParagraphLabel -count=5
go test github.com/egoist/mygo/cmd/mygo -run 'Test(DevConfigMacOSIcon|DevSessionMacOSICNS|BuildInputsMacOSIcon)$' -count=5
go run ./scripts/check-window-state -cycles 20
```

Linux 目标机可运行 `go test github.com/egoist/mygo/internal/linux -run TestLayerDrag`。
实际 Wayland 桌面拖动、macOS 图标及 GPU 渲染仍需在对应平台验收。
