# MyGo 本地补丁

当前依赖为 [MyGo v0.8.3](https://github.com/egoist/mygo/releases/tag/v0.8.3)。
以下补丁均已应用到受 Git 跟踪的 `vendor/github.com/egoist/mygo`，
路径相对 MyGo 根目录，并以干净的 v0.8.3 为基准生成。

| 补丁 | 用途 |
| --- | --- |
| `mygo-v0.8.3-radix-color-swatches.patch` | ColorPicker / ColorWell 内置色块使用 Radix Colors 第 9 阶与黑色端点，保留自选色及透明度。 |
| `mygo-v0.8.3-linux-layer-drag.patch` | Wayland layer-shell 歌词面板通过更新锚点拖动，限制在当前输出内，并通知位置保存。 |
| `mygo-v0.8.3-macos-dev-icon.patch` | 开发包优先选择 `macos.icon` 的 `-dev` 兄弟文件，正确读取和缓存 ICNS，并监听文件或资产目录变化。 |
| `mygo-v0.8.3-windows-state-compat.patch` | 将旧版本地补丁保存的 `windowsBounds` 读入上游 `pixel*` 字段；已有 `pixel*` 优先，下次保存使用上游格式。 |
| `mygo-v0.8.3-tester-label-target.patch` | Tester 点击同名目标时逐个验证实际命中，避免 RichText 父文本误挡内联链接；仍拒绝被遮挡的目标。 |
| `mygo-v0.8.3-desktop-audio.patch` | 保留桌面原采样率输出、可选设备预热、自然尾音排空及停止后端重开、ALSA 转换回退，以及 WSOLA 按输出率步进的修复与回归。 |
| `mygo-v0.8.3-analytic-clip.patch` | 单个未变换圆角直接由 GPU 裁剪变换后的内容，避免播放页整窗遮罩；分数硬边界、多圆角、变换裁剪及图片/渐变遮罩继续走上游实现。 |

重新生成 vendor 后，在项目根目录应用全部补丁：

```powershell
go mod vendor
Get-ChildItem patches/mygo-v0.8.3-*.patch | ForEach-Object {
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
go test github.com/egoist/mygo/audio -count=5
go run ./scripts/check-window-state -cycles 20
```

Linux 目标机可运行 `go test github.com/egoist/mygo/internal/linux -run TestLayerDrag`。
实际 Wayland 桌面拖动、macOS 图标及 GPU 渲染仍需在对应平台验收。

## 播放页裁剪内存

v0.8.3 已包含上游的[增长遮罩复用](https://github.com/egoist/mygo/pull/1428)、
[容量预算修复](https://github.com/egoist/mygo/pull/1408)和
[Windows 闲置纹理清理修复](https://github.com/egoist/mygo/pull/1394)，
替代本地提交 `da4d659` 的缓存和纹理修复。原 `clip-mask-memory.patch`
已移除，仍有效的单圆角解析裁剪优化独立为 `analytic-clip.patch`。

上游沿用像素遮罩，按轴翻倍复用增长尺寸、收缩过大的遮罩，并按实际容量
限制空闲预算。`TestGrowingClipKeepsItsMask` 直接覆盖从封面展开到整窗的
40 帧动画，同时检查像素覆盖和纹理尺寸复用。上游测试未被 `go mod vendor`
复制；前两条命令验证未打本地补丁的固定版本模块，后两条验证本地补丁和应用：

```powershell
go test -mod=mod github.com/egoist/mygo/internal/gpu -run 'Test(GrowingClipKeepsItsMask|IdleMasksStayWithinBudget|TurningCardMasksKeepWithinBudget)$' -count=5
go test -mod=mod github.com/egoist/mygo/internal/gpu/d3d11 -run 'Test(SweepBetweenUnchangedFrames|TransientCanvas)$' -count=5
go test github.com/egoist/mygo/internal/gpu github.com/egoist/mygo/internal/gpu/d3d11 -run 'Test(StationaryRoundedClipUsesFrameCoordinates|ComplexClipRetainsCoverageMask|RoundedCardClipsTransformedContentWithoutMask)$' -count=5
go test ./internal/app -run 'Test(NowPlaying|SpaceAfterOpeningNowPlaying|BackdropFadeEnds)' -count=5
```

Windows 原生离线十次开关使用同一份应用和探针，仅切换 MyGo 源码；
固定 600px 封面、60 行歌词、暂停、关闭 Fog，各采样先执行 GC：

| MyGo 实现 | 首次展开工作集 | 第十次展开工作集 |
| --- | --- | --- |
| 升级前 v0.8.1 完整本地补丁 | 125 MiB | 157 MiB |
| v0.8.3 上游 | 222 MiB | 321 MiB |
| v0.8.3 + 解析裁剪补丁 | 182 MiB | 229 MiB |

精简补丁保留了部分收益，仍未达到升级前的内存水平。它只改变 `prepare`
的单圆角路径，保留上游的裁剪标记、遮挡判断、复杂遮罩复用和后端清理。
这些数值是本机诊断场景的采样，不能当作所有机器的峰值或整应用承诺。
Windows Direct3D 像素回归已验证；GL / Metal 尚未在对应平台真机验收。
