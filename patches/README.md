# MyGo 本地补丁

## v0.7.4：颜色选择器使用 Radix 预设

`mygo-v0.7.4-radix-color-swatches.patch` 已应用到 vendor，将原生
`ColorPicker` / `ColorWell` 的内置色块替换为 Radix Colors 第 9 阶与黑色端点，
与应用配色约定一致。色相、透明度和 HEX 自选功能保持原样。

重新生成 vendor 后，在项目根目录重新应用；已应用时可用 `--reverse --check` 检查：

```sh
git apply --check --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-radix-color-swatches.patch
git apply --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-radix-color-swatches.patch
```

## v0.7.4：Linux layer-shell 面板拖动

`mygo-v0.7.4-linux-layer-drag.patch` 已应用到 vendor。layer-shell
面板不能使用普通 Wayland 顶层窗口的系统拖动；此补丁改为更新面板锚点，
并通知窗口位置变化，以便现有窗口状态机制保存位置。拖动限制在当前输出内。
重新生成 vendor 后，可在项目根目录重新应用：

```sh
git apply --check --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-linux-layer-drag.patch
git apply --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-linux-layer-drag.patch
```

Linux 后端拖动状态测试：`go test ./vendor/github.com/egoist/mygo/internal/linux -run TestLayerDrag`。
实际合成器的拖动与位置恢复仍需在目标 Wayland 桌面验证。

## v0.7.4：Windows 混合 DPI 窗口恢复

`mygo-v0.7.4-windows-window-state.patch` 已应用到受 Git 跟踪的
`vendor/github.com/egoist/mygo`。问题见本仓库 issue #4：副屏缩放不同于主屏时，
恢复窗口将保存的 DIP 坐标作为物理坐标选屏，退出后保存错误位置，反复启动逐次漂移。

补丁为 `StateKey` 额外保存正常窗口的物理像素矩形。恢复时按物理矩形确定显示器、
校验可见性，并按该屏当前 DPI 恢复 DIP 尺寸。创建 HWND 时直接使用恢复矩形，
使窗口从目标屏幕的 DPI 开始，避免先在主屏创建再移到副屏，触发
`WM_DPICHANGED` 对已转换的尺寸再次缩放。公共屏幕坐标 API 保持现有约定。
旧记录缺少可靠的物理矩形，保留尺寸及最大化/全屏状态并回退居中；普通窗口采集后
写入新格式。断开的屏幕或不可达位置也回退居中。

首版补丁遗漏了创建时的第二次 DPI 缩放。用户在 200% / 175% 配置确认窗口逐次
缩小，原生检查也复现宽度 1575 → 1378 → 1205 像素。当前 v2 补丁修复这一点；
首版已经保存的较小尺寸会保留，使用新版时先手动调回一次需要的大小。

重新生成 vendor 会覆盖补丁。依赖仍为 v0.7.4 时，在项目根目录重新应用：

```sh
git apply --check --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-windows-window-state.patch
git apply --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-windows-window-state.patch
```

已经应用时，可验证 vendor 与补丁一致：

```sh
git apply --reverse --check --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-windows-window-state.patch
```

补丁路径相对 MyGo 仓库根目录，后续可用于上游讨论或 PR。升级 MyGo 时需要重新
核对上游实现；上游合入后移除此补丁。当前尚未提交上游 PR。

Windows 下运行回归检查（设置 `CGO_ENABLED=0`）：

```sh
go test ./vendor/github.com/egoist/mygo ./vendor/github.com/egoist/mygo/internal/windows -count=5
go test ./internal/...
go tool mygo vet .
go run ./scripts/check-window-state -cycles 20
```

回归覆盖混合 DPI、实际 3840×2160@200% / 2560×1600@175% 布局、负坐标、
跨屏窗口、原点、DPI 变化、屏幕断开、旧记录回退、正常窗口矩形保留，以及
125% / 175% 副屏各连续恢复 20 次。原生检查现已在真实 200% / 175% 配置通过：
两屏分别连续创建、保存、恢复 20 次，位置、像素尺寸及 DIP 尺寸完全一致。
`check-window-state` 使用内存状态库与隐藏窗口；缺少混合 DPI 显示器时明确跳过，
默认以退出码 2 表示条件不满足。该检查不访问用户窗口配置，不修改系统缩放。

Windows v2 测试程序位于 `build/dpi-fix-v2/lilt.exe`，完整应用仍待用户验收。

## v0.7.4：macOS 开发版图标选择

`mygo-v0.7.4-macos-dev-icon.patch` 已应用到 vendor。`devConfig` 为
`macos.icon` 优先选择同目录、同扩展名的 `-dev` 兄弟文件，例如
`icon/AppIcon.icns` → `icon/AppIcon-dev.icns`；不存在时保留正式版路径。
通用 PNG 图标选择保持原有行为，原始配置对象和配置文件均不被修改。
开发包直接读取选中的 `.icns`，缓存随其路径、大小和修改时间更新；
未使用 ICNS 覆盖时，继续由通用 PNG 生成图标。

构建输入同时监听 macOS 正式版图标和预期开发版路径，即使开发版尚未创建，
也能检测其创建、修改和删除。`.icns` / `.car` 按文件监听，Icon Composer
的 `.icon` 按目录树监听。补丁包含独立的 `cmd/mygo/dev_icon_test.go`，
不依赖 vendor 已省略的上游测试或辅助函数。

补丁路径相对 MyGo 仓库根目录，可应用到干净的 v0.7.4。重新生成 vendor 后，
在项目根目录执行：

```sh
git apply --check --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-macos-dev-icon.patch
git apply --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-macos-dev-icon.patch
```

已应用时检查 vendor 与补丁一致：

```sh
git apply --reverse --check --directory=vendor/github.com/egoist/mygo patches/mygo-v0.7.4-macos-dev-icon.patch
```

Windows PowerShell 针对性验证：

```powershell
$env:CGO_ENABLED = "0"
go test ./vendor/github.com/egoist/mygo/cmd/mygo -run 'Test(DevConfigMacOSIcon|DevSessionMacOSICNS|BuildInputsMacOSIcon)$' -count=5
```

测试覆盖 ICNS、资产目录、Icon Composer 目录、绝对路径、扩展名大小写、缺失回退、
不同扩展名不误选、源配置不变，以及图标文件和目录内容的监听快照变化。
`TestDevSessionMacOSICNS` 还验证开发 ICNS 的实际读取、内容更新使缓存失效、
切回正式版路径及 PNG 回退。
