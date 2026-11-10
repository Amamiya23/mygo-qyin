# 音频 PR 评审修订记录

对应维护者对 `4fe548465fd2ec42e8ee956a8e9ee8029cc868fc` 的评审。

## 2026-11-09：正常退出时平滑关闭音频

为任务栏/托盘菜单“退出”与设置为“退出应用”后的窗口关闭增加统一音频清理。
“隐藏到托盘”仍继续播放。保存设置与播放位置的流程保留；音频清理接在事件循环
停止后的 App.OnQuit，也覆盖框架正常重启前的退出回调，不在可取消的退出前置回调中
提前关闭音频。强制杀进程、崩溃不在保证范围内。

新增桌面终止性 API `audio.Shutdown(ctx)`，拒绝后续 Play，
由输出线程按当前真实输出率播放当前歌曲接下来的 100ms 连续采样并逐帧淡出，排空并关闭会话。
音源延迟至淡出完成或输出失败后关闭，不在请求退出时提前关闭解码器。声音在此期间保留
活跃生命周期，避免 Player 的垃圾回收清理提前关闭音源。已排队但尚未在当前会话播放的
新歌不参与淡出；混音步进与常规 5ms 音量渐变显式使用当前会话采样率，避免退出恰逢切频时
误用新目标速率。切歌原有的 5ms 最后电平收尾不变。
关闭完成才通知等待者，不重开设备，不新增预热，也不等待 5 秒空闲超时。
完整提前量的静音已经输出后，暂停的会话直接关闭；从未打开设备时立即完成。
Web、Mobile 与其他不受支持的后端不受此桌面 API 影响。

应用退出的等待上限为 1 秒；上下文取消只结束等待，不跨线程关闭正在进行原生操作的
句柄，输出线程仍可继续清理。应用记录清理错误或超时并继续退出，避免异常设备令
进程长期残留。这是尽力完成的正常退出，不承诺异常硬件上的无爆音。

初版 5ms 电平归零的用户试听共 5 次（2 次托盘退出、3 次关闭主窗口退出），其中一次仍感觉
音乐突然停止，但没有额外杂音。用户随后选择 100ms 真实音乐淡出，以改善听感；这不是把
最后采样值的归零包络延长到 100ms。安装 100ms 版后用户反馈“好些了”，并指定
《ライアーコンプレッション》— 小玉ひかり作为此退出场景的固定试听曲目。
尚未记录具体退出时间点、该版本音源的采样率和重复测试次数；当前结论为主观听感改善，
不表述为完全消除截断或所有设备通过。后续对比应固定歌曲版本、相近播放位置、输出设备和音量，
分别检查托盘退出与配置“退出应用”后的主窗口关闭。

新增回归覆盖六档采样率、40ms/250ms 提前量下连续变化的立体声采样按精确 100ms 包络输出、
清理后不重开、从未打开、打开中退出、暂停且已经静音、取消等待、并发等待及原生
close 尚未返回时不得提前通知完成。测试通过通道同步，不依赖 Sleep。
还覆盖预热中退出不读取歌曲、切频中退出只淡出原会话声音且步进正确，以及短音源在淡出中
自然结束不触发 OnEnd。192/384kHz 是假设备测试，不代表实机验证。
应用测试通过真实设置控件选择“退出应用”，验证隐藏、退出、无托盘回退和退出中
关闭窗口不重复发起 Quit。Linux 386 目标验证非桌面 Shutdown 无操作，Web 测试编译
检查同一约束。退出行为已有上述本机试听反馈，其他设备、后端和完整重复测试仍待补充。

验证结果：音频包完整回归连续 5 次、音频 race 连续 3 次、完整项目测试和 MyGo vet
通过；Linux amd64、Windows amd64、macOS amd64 应用构建，macOS arm64/Web 音频测试
编译，以及 Linux 386 非桌面策略运行测试通过。桌面音频补丁重新应用后的 23 个文件
与 vendor 一致。已按项目打包流程生成本机 0.4.2 测试包，未安装或改动用户设置。

## 2026-11-08：0.4.1 手动跨采样率切歌的旧歌残留

用户在 USB DAC 上手动切歌时听到短促的前曲片段；补充测试中，48kHz 自然播完
切换至 44.1kHz 未出现同样现象。本次针对手动切歌的软件残留路径修复，不将该现象
泛化为自然切歌排空失败，也不把假设备复现表述为实测 DAC 爆音。

播放器调用层的 stop 先 Pause，100ms 后才 Close。Pause 的 5ms 淡出由混音器推进。
若下一首使设备提前切频，旧播放器仍可能处于活跃列表中；预热会暂停所有声音的源游标，
因此旧淡出可能在新会话的静音结束后才继续，混入前曲片段。200ms 静音按输出帧计数，
后端还有提前缓冲，不能用 100ms 的关闭定时器推断旧播放器一定会在静音混完前关闭。

修复在建立新输出会话时、持有音频状态锁的范围内，将已暂停声音的遗留淡出从活跃列表
移除并清零增益状态；不关闭或 seek 其音源，不推进游标，保留再次 Play 的正常淡入。
仍在播放的声音（含普通音效、自然 EOF 待排空的尾音）保持原样。同一会话内的暂停与
同采样率手动切歌仍使用原有 5ms 淡出，不增加预热；自然结束的排空、ALSA 回退和
按 SetPreRoll 配置执行设备冷开/重开预热的策略均保留。

新增 `TestManualRateSwitchDoesNotReplayPausedTail` 使用不同声道区分新旧歌曲，
将旧 Player 的 Close 延迟至新输出写入音乐后，避免依赖真实计时。覆盖
48→44.1、44.1→48、88.2→96、96→44.1kHz，以及 40ms/250ms 延迟、预热开/关。
未修复时 16 个用例全部失败：例如 48→44.1kHz 时，纯右声道的新歌曲在预热后
出现约 0.4977 的左声道旧歌曲采样。修复后要求新会话只有配置的静音与新曲精确首帧，
旧源不前进、手动切歌不等待源 EOF、暂停播放器仍可正常恢复并淡入。
另用 `TestManualSameRateSwitchKeepsPauseRamp` 验证同采样率切歌的原有淡出不被删除。

本轮验证结果：音频包完整回归 `-count=5`、音频竞态检查 `-race -count=3`、
`CGO_ENABLED=0 go test ./...` 和 `go tool mygo vet .` 均通过；Linux amd64、
Windows amd64、macOS amd64 应用构建，以及 macOS arm64 / Web 音频测试编译通过。
当前 PipeWire PulseAudio 兼容服务上的四档原生静音流生命周期检查通过；该检查仅覆盖
打开、写入、排空、复用、关闭，不验证音乐听感。同步更新 v0.8.3 桌面音频补丁，
在由原补丁还原的上游文件上重新应用后，21 个文件均与当前 vendor 一致。

用户安装前一版快速切换修复后反馈本机 USB DAC 手动切歌测试通过：不同采样率歌曲切换时，
原先短促的旧曲残留问题消失，旧歌可能直接截断。这符合主动跳过的策略：不等待旧曲
排队音频全部播完，重开后不继续补播旧淡出。自然播完的排空策略不变，不应截断尾音。
本次反馈未逐项列明试听的采样率组合、歌曲与次数，不据此宣称完整硬件测试矩阵通过。

验证边界：自动化测试确认应用交给新输出的数据没有该旧曲片段，用户试听确认了
报告问题设备上的修复效果；这些结果不保证其他 USB DAC、系统音频服务或后端
完全无切频爆音，也不替代 macOS、Windows 和直连 ALSA 的对应音乐听测。

### 经用户选择增加平滑切频收尾

用户随后选择接受额外的短暂等待，以减轻手动切频时的生硬截断。当前实现会记录最后
成功提交的立体声电平；在非自然结束的采样率切换时，按旧输出率生成约 5ms 的线性
归零包络，并确认旧队列和该包络播放完毕后关闭会话。它不回放旧歌曲片段、不继续
读取旧源，也不在旧设备消费下一首。只等待已排队音频及收尾，不等待整首歌 EOF。
通常增加约配置的 40ms/250ms 提前量加 5ms，实际等待由系统服务及设备决定。
输出队列排空期间仍不持有音频状态锁。

这是对前一版“主动切频立即关闭、不等待排队输出”的体验调整，经用户明确选择；
自然 EOF 排空、停止后端重开与 SetPreRoll 策略继续保留。同采样率切歌不走该路径。
前曲已经自然排空时不再补人工收尾，即使并行音效仍在播放；同一会话继续播放另一首
同采样率歌曲后，之后的手动切频仍可正常平滑收尾。尚未输出过音乐的静音会话或无排空能力的后端直接关闭，
不会为此增加等待。

回归额外检查旧设备 5ms 包络的逐帧数值、零终点、源游标不变和新设备精确首帧；
44.1/48/88.2/96/192/384kHz 的收尾每次写入都不超过 AudioQueue 的单缓冲 512 帧容量，
高采样率时拆成多次写入。这里的 192/384kHz 是假设备测试，不是实机切频验证。
平滑版的音频完整回归连续 5 次、竞态检查连续 3 次、完整项目测试、MyGo vet、
Linux/Windows/macOS 应用构建以及 macOS arm64/Web 音频测试编译均通过。
安装包二进制一致性、SHA256 校验和安装脚本语法检查通过；桌面音频补丁重应用后
21 个文件与当前 vendor 一致。
用户安装本次平滑版后反馈“新版情况好多了”，表明在其本机 USB DAC 的试听中，
相比直接关闭旧输出，平滑收尾进一步改善了切换体验。该反馈未量化残余爆音或截断，
不据此宣称完全消除所有切换问题，也不推广为其他设备及后端均已通过听测。

## 同步维护者 main

已获取并合并维护者 main 的 `9bfa7986bbc54e24a618bfa0adad8e835f47bb83`。
Now Playing 时间刷新修复由该提交独立提供。本地同名测试文件仅有注释差异，
合并时保留维护者版本，未另写重复的时间刷新修复。

## 1. 自然切歌保留前曲尾音

读到源 EOF 时，桌面 voice 进入待播放完毕状态；此时保持 Playing 为 true，暂不
触发 OnEnd。输出线程在主音乐自然结束，或所有剩余声音均自然结束时排空旧队列：

- PulseAudio：`pa_simple_drain`。
- ALSA：`snd_pcm_drain` 播完后停止 PCM，随后关闭旧会话，下一次播放重开并遵循 SetPreRoll 配置预热。
- WASAPI：等待 `GetCurrentPadding` 为零后再允许自然结束通知。
- AudioQueue：必要时启动短缓冲，使用 AudioQueueStop(false) 并读取 IsRunning 直到归零；确认尾音播完后关闭旧队列，下一次播放重开输出并遵循 SetPreRoll 配置预热。Stop 也可能停止音频硬件，不能将下一次启动视为持续运行。缓冲回收回调不等于播放完成。

排空在输出线程执行，等待期间不持有播放器状态锁。排空完成才通过输出时钟确认
尾音已播放并通知 OnEnd。用户主动 Pause、Stop、Close 可以取消待自然结束状态；
主动跳过后不等待源 EOF；当前平滑切频策略会先完成旧输出的短收尾及排队输出，再关闭旧会话。

AudioQueue 处理依据 Apple 对 [OutputCallback](https://developer.apple.com/documentation/audiotoolbox/audioqueueoutputcallback)、
[非立即停止](https://developer.apple.com/documentation/audiotoolbox/audioqueuestop(_:_:)) 和
[IsRunning](https://developer.apple.com/documentation/audiotoolbox/kaudioqueueproperty_isrunning)
的说明：回调只表示缓冲可复用，不能作为播放结束的判断。

回归测试沿用评审复现中的假设备及通道同步方式，验证 40ms/250ms 延迟下
44.1 → 96 → 48 → 88.2 → 192kHz 连续播放时，每次关旧设备前 Position 已达 Duration。
同时覆盖原生排空接口、无排空接口时的帧时钟回退、同采样率复用及主动切歌不等待自然 EOF。

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
| 同采样率连续切歌，后端排空后仍保持开启 | 否 |
| 同采样率从头重播，设备保持开启 | 否 |
| 短暂停恢复，设备保持开启 | 否 |
| 长暂停期间设备已关闭，再恢复 | 是 |

自然切歌新增排空保护，但不把每首歌当成冷启动。原有预热帧数、首帧保留、
延迟补偿和长短暂停测试保留，并更新同采样率重播/换曲的预期。

macOS 自然排空会停止 AudioQueue，直连 ALSA 的排空也会停止 PCM，因此这两个后端在下一次播放重开输出；这条路径只在播放器启用 SetPreRoll 时预热。维护者补充的回归测试分别验证预热开启与关闭时，旧尾音完整、旧会话先关闭、下一首首帧保留，以及仅开启预热时输出 200ms 静音。该测试模拟停止后端，不替代 macOS 或直连 ALSA 实机听测。

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
