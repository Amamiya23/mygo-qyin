# MyGo 本地补丁

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
