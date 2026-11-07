# 音频 PR 评审修订记录

对应维护者对 `4fe548465fd2ec42e8ee956a8e9ee8029cc868fc` 的评审。

## 同步维护者 main

已获取并合并维护者 main 的 `9bfa7986bbc54e24a618bfa0adad8e835f47bb83`。
Now Playing 时间刷新修复由该提交独立提供。本地同名测试文件仅有注释差异，
合并时保留维护者版本，未另写重复的时间刷新修复。

## 1. 自然切歌保留前曲尾音

读到源 EOF 时，桌面 voice 进入待播放完毕状态；此时保持 Playing 为 true，暂不
触发 OnEnd。输出线程在主音乐自然结束，或所有剩余声音均自然结束时排空旧队列：

- PulseAudio：`pa_simple_drain`。
- ALSA：`snd_pcm_drain`，随后 `snd_pcm_prepare`，允许同采样率继续复用。
- WASAPI：等待 `GetCurrentPadding` 为零后再允许自然结束通知。
- AudioQueue：必要时启动短缓冲，使用 AudioQueueStop(false) 并读取 IsRunning 直到归零；之后回收缓冲并保留队列对象和格式供下一首重新 prime/start。缓冲回收回调不等于播放完成。

排空在输出线程执行，等待期间不持有播放器状态锁。排空完成才通过输出时钟确认
尾音已播放并通知 OnEnd。用户主动 Pause、Stop、Close 可以取消待自然结束状态；
主动跳过后更改采样率仍走关闭旧会话的路径，不额外等待自然排空。

AudioQueue 处理依据 Apple 对 [OutputCallback](https://developer.apple.com/documentation/audiotoolbox/audioqueueoutputcallback)、
[非立即停止](https://developer.apple.com/documentation/audiotoolbox/audioqueuestop(_:_:)) 和
[IsRunning](https://developer.apple.com/documentation/audiotoolbox/kaudioqueueproperty_isrunning)
的说明：回调只表示缓冲可复用，不能作为播放结束的判断。

回归测试沿用评审复现中的假设备及通道同步方式，验证 40ms/250ms 延迟下
44.1 → 96 → 48 → 88.2 → 192kHz 连续播放时，每次关旧设备前 Position 已达 Duration。
同时覆盖原生排空接口、无排空接口时的帧时钟回退、同采样率复用及主动切歌不排空。

## 2. ALSA 不支持源格式时回退

恢复 `soft_resample=1`。先尝试 ALSA default PCM；如果该 PCM 不能接受源格式，
再尝试 `plug:default`，显式使用 ALSA 格式/采样率转换插件。两次打开都失败时才
返回错误，保留两次失败原因。应用流仍使用请求的源采样率，因此 mixer、进度补偿
与预热帧数无需误用硬件采样率。

测试模拟仅支持 48kHz 的直接 PCM 拒绝 96kHz 请求，检查 default 失败后关闭，
改用 plug:default 成功，并向返回的 ALSA device 写入音频帧。另覆盖
44.1/48/88.2/96/192kHz 请求时允许软件转换的参数。这是绑定 API 的假设备测试，
不是对真实受限 ALSA 硬件的验证。

## 3. Play / Duration 竞态

移除 Play 对 voice.rate 的重复赋值。源速率与长度在构造时确定，此后不可变；
Play 的采样率决策、设备重开时的元数据读取仍保留。Duration 不再与 Play 形成
未同步读写。并发回归持续执行 Pause/Play 与 Duration，配合 race detector 验证。

## 4. 预热策略

框架仍默认关闭预热，Lilt 主音乐流仍显式开启，但开启后的触发条件现在仅为
设备实际冷开或重开：

| 操作 | 200ms 预热 |
| --- | --- |
| 首次打开设备 | 是 |
| 改变采样率导致设备重开 | 是 |
| 同采样率连续切歌，设备保持开启 | 否 |
| 同采样率从头重播，设备保持开启 | 否 |
| 短暂停恢复，设备保持开启 | 否 |
| 长暂停期间设备已关闭，再恢复 | 是 |

自然切歌新增排空保护，但不把每首歌当成冷启动。原有预热帧数、首帧保留、
延迟补偿和长短暂停测试保留，并更新同采样率重播/换曲的预期。

## 验证命令

```sh
CGO_ENABLED=0 go test github.com/egoist/mygo/audio
go test -race github.com/egoist/mygo/audio
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go tool mygo vet .
go build .
GOOS=windows go build .
GOOS=darwin go build .
CGO_ENABLED=0 GOOS=js GOARCH=wasm go test -c -o /tmp/mygo-audio-review-web.test.wasm github.com/egoist/mygo/audio
```

可选的宿主 PulseAudio/PipeWire 原生检查（只输出静音，不修改路由）：

```sh
MYGO_AUDIO_DEVICE_TEST=1 CGO_ENABLED=0 go test github.com/egoist/mygo/audio -run '^TestLinuxPulseNativeLifecycle$' -v
```

本轮音频包、完整项目测试、音频 race detector、mygo vet、Linux/Windows/macOS
构建和 Web 音频测试编译均通过。截尾、ALSA 回退、长短暂停及 Duration 并发等
针对性测试连续运行 5 次通过；维护者提取的 TestNowPlayingTimesWithoutInput
也以 -count=5 通过。并行音效下主音乐排空、主动跳过取消待自然结束状态的补充
测试通过音频包常规和 race 检查。

本机环境为 AlmaLinux 10.2 x86_64、PipeWire 1.4.11 的 PulseAudio 兼容服务，
默认 sink 为 sof-hda-dsp 内置扬声器，服务器默认格式为 float32le / 2ch / 48000Hz。
原生检查实际完成 44.1/48/88.2/96kHz 应用流的打开、写入静音、排空、排空后继续写入
及关闭，四个子测试均连续 3 次通过。它不测量 DAC 时钟，也不证明音乐尾音或开头没有被
硬件静音；这些结论不能从应用流接受某个采样率直接推出。

## 尚未完成的实机验证

首次验证时未连接目标 USB DAC。随后已接入下文记录的 JCALLY USB DAC，完成
静音流生命周期与四档设备采样率观测。以下听测、回采或其他后端项目仍待补充：

- 冷启动、长暂停恢复时是否仍有硬件吞音或爆音。
- 跨采样率连续播放时，最后约 250ms 音乐及下一首开头是否完整。
- 真正缺少 PulseAudio 服务且硬件只支持有限采样率时的 ALSA 回退。
- Windows WASAPI 和 macOS AudioQueue 的原生运行行为。

共享 WASAPI、PipeWire 或 CoreAudio 仍可能重采样；本修订不实现独占输出，也不
承诺 DAC 硬件切频或端到端 bit-perfect。假设备测试、原生静音流检查和真实音乐/
目标 DAC 验证在本记录中分别列明。

## 2026-11-07 四档采样率追加验证

按 44.1、48、88.2、96kHz 扩展了原生静音流、输出延迟补偿、长暂停恢复、
自然切歌和 ALSA 回退测试。ALSA 的 48kHz 测试检查直接使用 default；其余
三档在模拟硬件仅支持 48kHz 时检查 plug:default 转换回退。

| 应用流请求 | pactl 实际报告的应用流格式 | 原生打开/写入/排空/复用 | ALSA 设备运行率观察 |
| --- | --- | --- | --- |
| 44.1kHz | float32le / 2ch / 44100Hz | 连续 3 次通过 | 48000Hz |
| 48kHz | float32le / 2ch / 48000Hz | 连续 3 次通过 | 48000Hz |
| 88.2kHz | float32le / 2ch / 88200Hz | 连续 3 次通过 | 48000Hz |
| 96kHz | float32le / 2ch / 96000Hz | 连续 3 次通过 | 48000Hz |

测试采用内置 sof-hda-dsp 声卡与 PipeWire 的 PulseAudio 兼容服务，未修改系统
路由、采样率配置或音量，只发送静音。运行时读取 pactl 的 sink-inputs，确认
本测试的 audio.test 流格式；同时读取 /proc/asound/card0/pcm0p/sub0/hw_params，
四档均显示 S32_LE、2 声道、48000Hz。应用流格式与设备运行率不同，说明这些档位
在当前共享音频配置下仍经过系统层采样率转换，不能表述为硬件完成了四档切频。

该轮音频包完整回归连续 5 次通过，音频 race 检查连续 3 次通过。实际设备测试只
验证静音流生命周期；预热、暂停恢复、首帧/尾音与进度行为由自动化假设备测试覆盖，
仍未完成目标 USB DAC 的音乐听测或回采测量。

复现命令：

```sh
CGO_ENABLED=0 go test github.com/egoist/mygo/audio -count=5
go test -race github.com/egoist/mygo/audio -count=3
MYGO_AUDIO_DEVICE_TEST=1 CGO_ENABLED=0 go test github.com/egoist/mygo/audio -run '^TestLinuxPulseNativeLifecycle$' -v -count=3
```

本地详细日志位于 build/audio-rate-validation/ 下的 regression.log、race.log、
native-pulse.log 和 native-observations.json。日志目录沿用 build/ 的 Git 忽略规则。

## 2026-11-07 USB DAC 四档追加验证

用户接入 JCALLY Company CS43131+SGM8262 HIFI Audio，系统识别为
USB-Audio card 1 / device 0，USB ID 为 262a:18c8。设备描述在 16/24/32-bit PCM
格式下均列出 44.1、48、88.2、96、176.4、192、352.8、384kHz 支持率。
本轮仅测试用户指定的前四档，不将描述中支持 384kHz 表述为已经通过 384kHz 实测。

环境仍为 AlmaLinux 10.2 x86_64、PipeWire 1.4.11 的 PulseAudio 兼容服务。
开始测试时默认输出已是该 USB DAC，clock.allowed-rates 为
[44100, 48000, 88200, 96000]，clock.force-rate 为 0。通过测试子进程的
PULSE_SINK 指向该 USB 输出；未修改默认路由、音量、PipeWire 配置或强制采样率。

| 应用流请求 | pactl 观察到的应用流格式 | USB ALSA hw_params | USB Momentary freq | 生命周期测试 |
| --- | --- | --- | --- | --- |
| 44.1kHz | float32le / 2ch / 44100Hz | S32_LE / 2ch / 44100Hz | 44100Hz | 连续 3 次通过 |
| 48kHz | float32le / 2ch / 48000Hz | S32_LE / 2ch / 48000Hz | 48000Hz | 连续 3 次通过 |
| 88.2kHz | float32le / 2ch / 88200Hz | S32_LE / 2ch / 88200Hz | 88200Hz | 连续 3 次通过 |
| 96kHz | float32le / 2ch / 96000Hz | S32_LE / 2ch / 96000Hz | 96000Hz | 连续 3 次通过 |

观测同时核对测试流已链接到该 USB sink，并读取
/proc/asound/card1/pcm0p/sub0/hw_params 及 /proc/asound/card1/stream0。
排除了切换时尚未绑定 sink、旧设备参数尚未更新等瞬态，保留连续观测到的状态及
最终匹配值。与之前内置声卡始终运行在 48kHz 的结果不同，本轮确认 USB 设备在
四档测试中均出现了与源请求一致的运行采样率及 USB 反馈频率。

每档原生检查实际执行打开、写入静音、排空、继续复用写入、再次排空、关闭；
全部四档连续运行 3 次通过。这证明了这些格式的应用流和设备运行采样率可以匹配，
不能仅据此断言样本值端到端 bit-perfect，也不能证明硬件未吞掉音乐开头或尾音。
本轮未播放音乐、未做耳听或数字/模拟回采，冷启动和长暂停恢复的实际吞音、爆音
仍保留为待验证事项。

本轮有效日志保留在 build/audio-rate-validation-usb/native-pulse-confirmed.log，
连续状态与匹配证据在 native-confirmed-observations.json，设备能力描述在
usb-capabilities.txt。同目录的首轮观测记录包含切换瞬态，不作为稳定运行率的结论。
