# 使用已保存的 QQ / 微信登录测试 Hi-Res

功能使用、降级与缓存机制、验证记录见 [Hi-Res：使用、实现与验证](hires.md)。本文保留可直接运行的测试命令。

此测试绕过播放器的音质回退，分别探测 `RS01`（Hi-Res 候选）和 `F000`（普通无损），对比应用当前的 `UrlGetVkey` 与 `CgiGetVkey` 通道，以及两种文件名拼接方式。它读取本机登录态，不刷新或改写令牌，不输出令牌或签名播放链接。只有显式启用时才联系 QQ 音乐。

## 运行

1. 先在 Lilt 中使用 QQ 或微信扫码登录，确认可以正常播放。若登录已过期，重新扫码。
2. 在官方 QQ 音乐中选一首当前账号可以用 Hi-Res 播放的歌曲，复制歌曲链接。其中 `songDetail/` 后的字母数字字符串就是歌曲 MID；不是专辑 MID、数字歌曲 ID 或分享短链。如果只有短链，先在浏览器打开，找到完整歌曲链接。
3. 在仓库根目录运行，将 `替换为歌曲MID` 换成实际值：

```bash
LIVE=1 LIVE_HIRES_ACCOUNT=1 LIVE_HIRES_SONG_MID='替换为歌曲MID' \
  go test ./internal/qqmusic -run '^TestLiveHiRes$' -count=1 -v -timeout=5m
```

默认从正式版 Lilt 的系统凭据存储读取 `qqmusic-session`，微信会话的 `loginType=1` 会被保留，连同设备及鉴权字段一起复用。无需获取浏览器 Cookie，也无需手动拼接微信 OpenID 或令牌。

如果通过 `go tool mygo dev` 登录，应用将凭据保存在用户数据目录中的 `qqmusic-session` 文件。可以指定该文件（不要复制文件内容到终端）：

```bash
LIVE=1 LIVE_HIRES_ACCOUNT=1 LIVE_HIRES_SONG_MID='替换为歌曲MID' \
  LIVE_HIRES_SESSION_FILE='/实际用户数据目录/qqmusic-session' \
  go test ./internal/qqmusic -run '^TestLiveHiRes$' -count=1 -v -timeout=5m
```

Linux 开发版的默认路径是 `${XDG_CONFIG_HOME:-$HOME/.config}/Lilt Dev/qqmusic-session`。macOS 开发版默认在 `~/Library/Application Support/Lilt Dev/qqmusic-session`，Windows 开发版默认在 `%AppData%\Lilt Dev\qqmusic-session`。若应用使用了自定义数据目录，请使用实际路径。

## 解读输出

- `loginType=1`：已加载微信登录态。
- `RS01 ... FLAC 24-bit 96000 Hz` 等：该请求实际取得了对应音源的 FLAC 文件头。测试要求 RS01 返回至少 24-bit、44.1 kHz。
- `F000` 成功、`RS01` 失败：当前歌曲、账号或请求通道未能取得 Hi-Res；换另一首官方客户端确认支持的歌曲复测，不能仅凭一次失败断言账号不支持。
- `empty purl` 或非零业务码：上游未提供播放地址，可能涉及登录、权限、歌曲资源或接口参数。
- 获取链接后出现 HTTP 403/404：CDN 未交付音频，不能当作成功。
- 测试 `PASS`：至少一个 RS01 候选的文件头符合上述条件。它只验证链接交付及音频元数据，不验证整首文件完整性、实际播放或声卡输出规格。

测试最多读取每个 CDN 响应的前 64 KiB，不保存音乐文件。可分享诊断输出；请勿分享会话文件、MusicKey、RefreshToken 或完整播放链接。

## 播放器接入与完整解码测试

设置页已提供 **Hi-Res（FLAC）** 选项，下一首生效。播放器先通过 `CgiGetVkey` 请求 `RS01`，检查实际交付的 FLAC 文件头；无权限、空地址、无效音频或 CDN 403/404 时回退到普通无损，再尝试 320k 和 128k。网络连接错误会作为错误报告。音质标识与缓存按照实际返回的音质记录。

下面的可选测试使用正式版 Lilt 的已保存登录，下载整首歌曲到测试临时缓存，验证应用解码器、拖动和缓存重播，不向声卡播放声音。歌曲数字 ID 可以从上面的探测输出 `song ID=` 读取；测试过程可能按应用既有逻辑刷新过期登录态。

```bash
LIVE=1 LIVE_HIRES_PLAYBACK=1 \
  LIVE_HIRES_SONG_ID=491303214 LIVE_HIRES_SONG_MID=001NJ5rE2EPEIv \
  go test ./internal/player -run '^TestLiveHiResPlayback$' -count=1 -v -timeout=5m
```

此示例歌曲约下载 81 MB，临时音乐文件随测试清理。测试不验证声卡输出规格；实际听感、暂停与恢复可在应用中切换到 Hi-Res 后检查。
