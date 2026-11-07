# 桌面音频输出说明

本项目对 `vendor/github.com/egoist/mygo/audio` 和 `internal/player` 的以下两项修改，
仅在受支持的 **Desktop 桌面后端**启用。平台限制已在代码中实现。

| 功能 | Windows / macOS / Linux | Web | Mobile（Android / iOS） |
| --- | --- | --- | --- |
| 动态跟随音源采样率输出 | 支持 | 不适用 | 不适用 |
| 200ms 硬件切频与唤醒静音预热 | 显式开启后支持 | 禁用 | 禁用 |

`policy_desktop.go` 与 `policy_other.go` 使用互补的 Go 构建约束选择平台能力：

- macOS：`darwin && !ios`。
- Linux：`linux && !android`，且为 `amd64` 或 `arm64`。
- Windows：`amd64` 或 `arm64`。
- Web、Android、iOS 和不支持的后端：固定使用原有 48000 Hz，不启用预热。

显式排除 Android 与 iOS 是必要的：Go 构建约束中 Android 同时满足 `linux`，
iOS 同时满足 `darwin`。Web 的 AudioContext 保持原有固定采样率、单实例复用方式，
不会因歌曲切换而销毁重建。

## 播放器 API 与默认行为

`NewPlayer`、`OpenStream`、`Sound.NewPlayer` 和 `Sound.Play` 创建的播放器默认
**不启用 200ms 预热**。界面点击等普通音效在冷启动、重播和短暂停恢复时，都不会
因本功能新增 200ms 静音；桌面动态采样率选择仍生效。

Lilt 在 `internal/player/player.go` 创建主音乐播放器时显式配置：

```go
out := audio.NewPlayer(decoder).
    SetPrimary(true).
    SetPreRoll(true)
out.Play()
```

`SetPrimary(true)` 让主音乐流优先决定设备采样率，避免短音效或旧歌曲淡出决定输出率。
`SetPreRoll(true)` 为该播放器请求 200ms 硬件预热，两者可独立使用。
在 Web、Android、iOS 和不支持的后端上，两项设置都不会启用桌面输出功能。
无需预热时保留默认值，或调用 `SetPreRoll(false)`。

选项在下一次开始、恢复或设备打开时按触发条件生效；已在播放的 Player 上再次
调用 `Play()` 仍是无操作。关闭预热后，如果没有其他正在播放的 Player 请求预热，
尚未输出的预热静音会在下一次混音时取消；停止、暂停或关闭最后一个请求者也会
取消剩余静音，避免影响之后的普通音效。

## 动态采样率输出

在桌面后端，每次播放或恢复播放时，从音源保存的原始采样率元数据选择输出采样率，默认初始值
仍为 48000 Hz。主音乐流可以决定输出采样率；采样率变化时主动关闭旧输出流，
按目标采样率重新打开设备，无需等待原有的 5 秒空闲关闭。

设备因空闲关闭后，恢复播放仍按当前音源的原始采样率打开，不回退到固定 48000 Hz。
当音源与输出采样率相同、播放速度为 1 倍时，混音步进为 1，跳过插值计算。

音源元数据来自 `Decoder.SampleRate()`，在解码工作线程启动前保存，播放和恢复
时重新读取该元数据，不会依赖可被重置的输出状态，也不会与解码线程并发访问 Decoder。

这消除了应用内固定 48000 Hz 的强制重采样。端到端 Bit-Perfect 仍取决于操作系统
音频服务、设备配置、音量和音量平衡等处理；当前共享音频后端不能保证硬件端始终
不发生重采样或其他信号处理。

## 200ms 硬件静音预热

仅在桌面平台、且有播放中的 Player 开启 `SetPreRoll(true)` 时，以下情况先输出
目标采样率下 `outputRate / 5` 帧全零静音，再消费音乐采样：

- 切歌、播放新 Player，或从头重新播放，即使采样率相同。
- 设备冷启动，或因切频、设备丢失等原因重新打开。
- 长暂停期间设备已空闲关闭，恢复播放时重新打开。

涉及重开设备时，先打开设备，再发送静音帧；预热不靠单纯等待 200ms 实现。
静音期间不推进音乐读取游标，保留歌曲或暂停恢复处的首帧，播放进度不增加 200ms。
同一首歌短暂停后恢复，若设备仍开启且采样率一致，则使用原有的 5ms 音量渐变，
不新增预热。判断依据是设备实际状态，而非仅比较暂停时长。

所有 Player 共用一个物理输出流。预热期间输出全零，所有混音源暂不推进；因此
音乐播放器触发预热时，同设备上已有或新加入的音效也会等待剩余的静音帧。
普通音效本身不会触发、延长或重新开始预热。这不是各 Player 独享设备的实现。

## 验证范围

桌面应用构建检查：

```sh
go build .
GOOS=windows go build .
GOOS=darwin go build .
CGO_ENABLED=0 go test github.com/egoist/mygo/audio
go test -race github.com/egoist/mygo/audio
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go tool mygo vet .
```

音频回归测试覆盖采样率切换、静音帧数、首帧保留、进度补偿、短暂停恢复和
5 秒空闲关闭后的恢复流程，还覆盖默认音效无预热、音效不重置音乐预热、关闭最后一个
请求者取消剩余预热。

非桌面策略回归可在 Linux x86_64 主机运行以下命令，实际执行 `policy_other.go`
分支，检查显式设置桌面选项后仍为 48000 Hz 且无预热：

```sh
CGO_ENABLED=0 GOARCH=386 go test github.com/egoist/mygo/audio
CGO_ENABLED=0 GOOS=js GOARCH=wasm go test -c -o /tmp/mygo-audio-web.test.wasm github.com/egoist/mygo/audio
```

386 测试需要宿主支持执行 32 位 Linux 程序；Web 命令只验证测试代码可编译，
不表示已经在浏览器运行。另通过 `go list` 核对 Linux、Windows、macOS、Web、
Android、iOS 实际选择的平台策略文件。Android/iOS 的策略文件选择检查不等同于
完整移动端构建或运行验证；本修改不新增移动音频后端，也不修复现有 purego/cgo
依赖及移动工具链限制。

交叉编译通过不等于已经在目标平台的真实 DAC 上验证硬件切频和锁定行为。
