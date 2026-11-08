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
| `mygo-v0.8.3-desktop-audio.patch` | 保留桌面原采样率输出、可选设备预热、自然尾音排空及停止后端重开、ALSA 转换回退、切频时隔离已暂停旧曲淡出并在旧输出平滑收尾，以及 WSOLA 按输出率步进的修复与回归。 |
| `mygo-v0.8.3-analytic-clip.patch` | 将可化简的静止几何裁剪直接交给 GPU，包含完整切除圆角的分数滚动边界及不透明冗余裁剪；重叠抗锯齿、变换及图片/渐变遮罩保留像素路径。 |
| `mygo-v0.8.3-render-resource-lifetime.patch` | 在预算压力下跨开关复用闲置遮罩，固定坐标裁剪保留可复用尺寸；D3D11 / GL / Metal 在完成绘制或提交后清理退场的临时遮罩纹理及离屏层。 |

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

以下为初版单圆角解析裁剪的验收记录；当前补丁的进一步化简见后文。
Windows 原生离线十次开关使用同一份应用和探针，仅切换 MyGo 源码；
固定 600px 封面、60 行歌词、暂停、关闭 Fog，各采样先执行 GC：

| MyGo 实现 | 首次展开工作集 | 第十次展开工作集 |
| --- | --- | --- |
| 升级前 v0.8.1 完整本地补丁 | 125 MiB | 157 MiB |
| v0.8.3 上游 | 222 MiB | 321 MiB |
| v0.8.3 + 初版解析裁剪补丁 | 182 MiB | 229 MiB |

初版精简补丁保留了部分收益，仍未达到升级前的内存水平。它只改变 `prepare`
的单圆角路径，保留上游的裁剪标记、遮挡判断、复杂遮罩复用和后端清理。
这些数值是本机诊断场景的采样，不能当作所有机器的峰值或整应用承诺。
Windows Direct3D 像素回归已验证；GL / Metal 尚未在对应平台真机验收。

## 播放页遮罩复用与临时资源回收

`render-resource-lifetime.patch` 补充播放页反复展开、收起时的复用和退场行为：

- 新建或扩容遮罩造成预算压力时，优先复用本帧未使用的闲置缓冲；本帧已被批次引用的遮罩不会覆盖。预算内的小型静态分支仍保留缓存。
- 固定坐标、无图片或渐变遮罩的裁剪保留纹理尺寸供收起及下次展开复用；viewport 缩小时夹紧，精确形状缓存命中也检查 viewport。变换、透视及图片/渐变遮罩保留上游的四分之一收缩规则。
- 生成的遮罩标记为 `Image.Transient`。三个 GPU 后端在完成构建/绘制或提交后释放退场的瞬态纹理、离屏 Canvas 及其 Builder。跳过重建的无变化帧保留当前资源；再次出现时可由 CPU 像素或 Canvas 场景恢复。
- 保留每 Builder 32 MiB 闲置 CPU 遮罩预算和普通图片原缓存策略，不新增 GPU 等待、不强制 GC、不修改动画或封面清晰度。瞬态遮罩间歇性退场后重现需要重新上传纹理。

新增回归可运行：

```powershell
go test github.com/egoist/mygo/internal/gpu -run 'Test(Reopened|IdleMaskReuse|ClipMask|MovingAndPainted)' -count=5
go test github.com/egoist/mygo/internal/gpu/d3d11 -run TestTransientResources -count=5
```

本轮 Windows 原生离线同场景十次开关（600px 合成封面、60 行歌词、暂停、
关闭 Fog、每次采样先 GC）记录在 `build/nowplaying-resource-reuse/`。
最终二进制的单轮对照如下；工作集有波动，不能当作所有机器的峰值：

| 指标 | 修改前 | 修改后 |
| --- | --- | --- |
| 首次展开工作集 | 172.6 MiB | 150.7 MiB |
| 首次收起工作集 | 184.3 MiB | 179.2 MiB |
| 第十次收起工作集 | 234.8 MiB | 229.2 MiB |
| 第十次收起 PrivateBytes | 410.8 MiB | 296.2 MiB |
| 第十次收起累计 TotalAlloc | 499.6 MiB | 320.1 MiB |

累计分配含启动及诊断采样自身，不是存活内存。工作集回落仍有限，不能宣称
收起后恢复到启动水平。完整应用测试、MyGo vet、D3D11 全套测试、三个桌面
平台构建和补丁重应用检查通过；GL / Metal 尚未在对应平台真机验收。

## 分数滚动边界与圆角裁剪化简

播放页剩余的大遮罩来自开合卡片的圆角与歌词滚动区域的分数硬边。
`analytic-clip.patch` 现进一步化简这些几何，使用原有 shader 的 `rect + radii`，
不修改 shader 或字节码，也不改变页面布局、动画和封面清晰度：

- 在当前整数 scissor 的整个像素方格中完全不透明的裁剪可忽略。
- 单个普通圆角与一条有效分数硬矩形相交时，只有被完整切掉的角才清零半径；保留角的影响区不能跨新矩形中点。
- 重复或近邻分数边、切入圆角曲线、薄条、多个同时作用的圆角仍使用像素遮罩。连续角的新合成保持保守，沿用原单圆角路径。
- 前两项遮罩复用与临时资源回收继续保留。相关测试改用确实相交的两个圆角，继续覆盖必要的像素路径。

三轮诊断中，面积至少 50,000 像素的回退从存在约 9.6 / 5 MiB 大遮罩变为
未出现；这不代表所有小遮罩都为零。诊断位于 `build/analytic-clip-audit/`。

正式对照复用同一未插桩离线探针：1180×780 请求窗口、600px 合成封面、
60 行歌词、暂停、关闭 Fog、十次开关，每个采样先 GC。
基线包含前两项优化；日志及逐阶段 pprof 在 `build/clip-reduction/{baseline,optimized}/`：

| 指标 | 本次化简前 | 本次化简后 |
| --- | --- | --- |
| 首次展开工作集 | 174.2 MiB | 94.5 MiB |
| 首次收起工作集 | 183.0 MiB | 97.6 MiB |
| 第十次收起工作集 | 245.0 MiB | 98.8 MiB |
| 第十次收起 PrivateBytes | 316.7 MiB | 141.7 MiB |
| 第十次收起累计 TotalAlloc | 287.5 MiB | 72.3 MiB |

累计分配含启动及诊断自身，不是存活内存；数值为单机离线场景采样，
不能承诺所有歌曲、窗口尺寸、显示缩放和设备都保持相同占用。

```powershell
go test github.com/egoist/mygo/internal/gpu -count=5
go test github.com/egoist/mygo/internal/gpu/d3d11 -count=5
```

新增共享回归覆盖可化简与必要回退两类，真实 D3D 像素对照覆盖 14 组几何、
每组 4 种内容变换；两者均连续 5 次通过。完整应用测试、MyGo vet、D3D11
全套测试、上游闲置预算及透视卡片回归、三个桌面平台构建均通过。
八份补丁在干净上游文件上逐份重应用，43 个受影响文件与当前 vendor 一致。
GL / Metal 真机 GPU 尚未验收。
