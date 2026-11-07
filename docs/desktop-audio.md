# 桌面音频输出说明

本项目对 `vendor/github.com/egoist/mygo/audio` 与 `internal/player` 进行了扩展，新增了 **动态跟随音源采样率输出** 与 **200ms 硬件静音预热（Pre-roll Silence）** 两项能力。

上述特性仅在受支持的 **桌面端后端（Windows / macOS / Linux）** 启用，其他平台保持原有行为不变：

| 功能特性 | Windows / macOS / Linux | Web | 移动端（Android / iOS） |
| --- | --- | --- | --- |
| 动态跟随音源采样率输出 | 支持 | 不适用 | 不适用 |
| 200ms 硬件切频与唤醒静音预热 | 显式开启后支持 | 禁用 | 禁用 |

### 平台构建约束

代码通过 `policy_desktop.go` 与 `policy_other.go` 中互补的 Go 构建标签（Build Constraints）区分平台策略：

- **macOS**：`darwin && !ios`
- **Linux**：`linux && !android`，且架构为 `amd64` 或 `arm64`
- **Windows**：`windows`，且架构为 `amd64` 或 `arm64`
- **Web、Android、iOS 及其他架构**：固定使用 `48000 Hz` 输出，不启用静音预热

> **说明**：在 Go 的构建标签规则中，`android` 会同时匹配 `linux`，`ios` 会同时匹配 `darwin`，因此必须显式排除移动平台。此外，Web 端的 `AudioContext` 保持原有的固定采样率与单实例复用机制，不会因切歌而销毁重建。

---

## 播放器 API 与默认行为

通过 `NewPlayer`、`OpenStream`、`Sound.NewPlayer` 和 `Sound.Play` 创建的播放器，**默认关闭 200ms 静音预热**。因此，界面点击等普通音效在冷启动、重播或暂停恢复时均不会引入额外延迟（桌面端的动态采样率输出正常生效）。

Lilt 在 `internal/player/player.go` 中初始化主音乐流时，通过链式调用显式开启上述特性：

```go
out := audio.NewPlayer(decoder).
    SetPrimary(true).
    SetPreRoll(true)
out.Play()
```

### 配置项说明

- **`SetPrimary(bool)`**：将当前播放器标记为主音频流，使其优先决定音频设备的输出采样率，避免短促的 UI 音效或正在淡出的上一首歌曲干扰输出采样率决策。
- **`SetPreRoll(bool)`**：为当前播放器启用 200ms 硬件静音预热。该选项可与 `SetPrimary` 独立使用；若无需预热，保持默认或调用 `SetPreRoll(false)` 即可。

在 Web、Android、iOS 等非桌面后端上，调用上述两个方法均为无操作（No-op），不会激活桌面端特性。

### 生命周期与取消机制

- 配置项会在下一次开始播放、恢复播放或打开音频设备时按条件生效；对已处于播放状态的 `Player` 重复调用 `Play()` 仍为无操作。
- 若在预热期间关闭预热选项，或停止、暂停、关闭最后一个请求预热的 `Player`，混音器会在下一轮混音周期内立即取消尚未输出的剩余静音帧，避免阻塞后续普通音效。

---

## 动态采样率输出

在桌面后端上，输出采样率的默认初始值为 `48000 Hz`。每次开始或恢复播放时，系统会根据主音乐流的音源采样率动态调整输出设备：

1. **区分自然结束与主动切歌**：自然结束时先等待旧音频缓冲排空，再通知 OnEnd 并切频；用户主动跳过、停止或关闭旧 Player 后仍可立即重连，无需等待 5 秒空闲超时。
2. **休眠唤醒保持采样率**：当设备因暂停或停止超过 5 秒而自动关闭后，再次恢复播放时仍会按当前音源的真实采样率重新打开设备，不会回退至 `48000 Hz`。
3. **线程安全的元数据读取**：音源采样率来自 `Decoder.SampleRate()`，在解码工作线程启动前即完成缓存；源速率字段构造后不再写入，播放与恢复时读取不可变值，既不依赖易变的输出层状态，也避免了与解码线程并发访问 `Decoder`。
4. **同采样率跳过插值**：当音源采样率与实际输出采样率一致且播放倍速为 `1.0` 时，混音步进精确为 `1`，直接跳过重采样插值计算。

> **关于 Bit-Perfect 的说明**：此改动消除了应用层内部强制重采样到 `48000 Hz` 的限制。但由于底层仍使用操作系统的共享音频服务（未开启硬件独占模式），且软件内的音量、声像（Pan）、变速与平衡调节仍会改变采样值，端到端是否发生重采样还取决于操作系统音频管线（如 PipeWire / WASAPI / CoreAudio）及硬件 DAC 的配置。

---

## 200ms 硬件静音预热

在桌面平台上，当存在开启了 `SetPreRoll(true)` 的活跃 `Player` 时，为防止硬件 DAC 在切频锁定锁相环（PLL）或从休眠唤醒期间吞掉歌曲开头，系统会在以下场景先输出 200ms（即 `outputRate / 5` 帧）的全零静音，再开始消费音乐采样：

- **同采样率切歌或重播**：设备持续开启时不新增预热，包括自然连续切歌和从头重播；仅当这次操作实际导致设备重开时预热。
- **设备冷启动或重连**：设备首次打开，或因采样率切换、设备重连而重新打开。
- **长暂停后唤醒恢复**：暂停期间底层音频设备已因空闲超时关闭，恢复播放并重新打开设备时。

ALSA 配置保留 soft_resample=1：请求源采样率，但允许 ALSA 插件在硬件不支持时转换。default PCM 拒绝格式时会再尝试 plug:default，显式启用转换层。混音器仍按应用流采样率工作；这不表示硬件使用了该采样率，也不保证 bit-perfect。

自然结束使用 PulseAudio pa_simple_drain、ALSA snd_pcm_drain（随后 prepare）、WASAPI padding 归零和 AudioQueue 非立即停止并等待 IsRunning 归零等待尾音播放完。并行音效下，主音乐的自然结束也会排空已排队输出；主动切歌不执行自然排空。Playing() 和 OnEnd 在自然结束时延后到尾音排空/播放时钟确认结束后，不再在混音读到 EOF 时提前通知。无原生排空能力的假设备用输出帧时钟等待。

### 核心机制细节

- **主动静音推送**：涉及设备重开时，系统会先以新采样率打开底层音频设备（触发系统与 DAC 切换时钟），随后写入静音帧，而非单纯通过休眠等待 200ms。
- **零进度损耗**：输出预热静音期间不会推进 `Player` 的音频读取游标，完整保留歌曲开头或暂停恢复处的首个采样帧，且 `Player.Position()` 播放进度不会产生 200ms 的偏移误差。
- **短暂停无延迟恢复**：同一首歌暂停后恢复时，系统会根据**音频设备的实际开启状态**进行判断——若设备尚未关闭且采样率一致，则直接通过原有的 5ms 音量渐变立即恢复播放，不触发 200ms 预热。
- **共享混音流行为**：所有 `Player` 共享同一个物理输出流。当主音乐流触发 200ms 预热时，混音器在预热帧耗尽前统一输出全零数据，同设备上的其他声音也会同步等待剩余静音帧；但普通音效自身不会触发、延长或重置预热计数。

---

## 测试与验证

### 1. 桌面端编译与回归测试

```sh
# 跨平台编译检查
go build .
GOOS=windows go build .
GOOS=darwin go build .

# 单元测试、竞态检查与静态分析
CGO_ENABLED=0 go test github.com/egoist/mygo/audio
go test -race github.com/egoist/mygo/audio
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go tool mygo vet .
```

自动化回归测试已覆盖以下场景：
- 多采样率切换、预热静音帧数精度、跨缓冲区边界处理、首帧保留及播放进度延迟补偿；
- 短暂停立即恢复、5 秒空闲关闭后唤醒恢复、设备开启过程中快速切歌；
- 普通音效默认无预热、音效混入不重置主音乐预热、关闭最后一个预热请求者自动取消剩余静音；
- 40ms/250ms 缓冲下连续跨采样率自然切歌不截尾、原生排空与帧时钟回退、主动跳过不排空、同采样率自然切歌不新增预热；
- 模拟仅支持 48kHz 的 ALSA 插件打开并写入 96kHz 应用流；并发 Pause/Play 与 Duration 的 race 检查。

### 2. 非桌面端平台策略验证

在支持运行 32 位程序的 Linux `x86_64` 主机上，可通过 `GOARCH=386` 触发并运行 `policy_other.go` 分支测试，验证非桌面策略下即使显式调用 `SetPrimary` / `SetPreRoll` 仍保持 `48000 Hz` 且无预热：

```sh
# 运行非桌面策略分支单元测试
CGO_ENABLED=0 GOARCH=386 go test github.com/egoist/mygo/audio

# 验证 Web (Wasm) 目标测试代码可正常编译
CGO_ENABLED=0 GOOS=js GOARCH=wasm go test -c -o /tmp/mygo-audio-web.test.wasm github.com/egoist/mygo/audio
```

此外，已通过 `go list` 核对 Linux、Windows、macOS、Web、Android、iOS 各目标平台均正确命中对应的 `policy_*.go` 文件。

### 3. 验证边界说明

- 上述命令中的 Web 目标仅验证 Wasm 编译通过，不包含浏览器实机运行测试；
- Android 与 iOS 仅验证构建约束文件选择，不包含移动端完整应用打包与运行测试；
- 交叉编译与单元测试通过不替代目标平台上真实物理 DAC 的硬件切频锁定耗时测量。

本次评审问题、对应修订、验证命令与尚未完成的目标 DAC 验证见[评审修订记录](audio-review-response.md)。
