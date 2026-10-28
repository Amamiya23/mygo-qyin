# MyGo Q音版

基于 MyGo 云音乐改造的 Q 音客户端。保留 Go 原生界面与播放器，以纯 Go 直接连接 Q音服务。
![截图1](assets\image.png)

## 功能

- 扫码登录，登录态保存在系统凭据存储中
- 每日推荐、推荐歌单、排行榜、我的歌单
- 搜索单曲、歌手、专辑，搜索建议与历史
- 歌手热门与最新歌曲、专辑及用户歌单
- QQ「猜你喜欢」电台
- 边下边播，按账号权限提供 128k / 320k MP3 / FLAC，逐级回退
- 听过的歌曲缓存在本机，再次播放无需等待
- 暂停与继续时淡出淡入，拖动进度无杂音
- 退出后再打开，从上次的播放队列与进度继续
- LRC 歌词，正在播放页的「律动」视图与歌曲评论
- 刘海播放器：在屏幕顶部刘海处显示迷你播放器，悬停展开（在设置中开启）
- 喜欢歌曲、歌单编辑、播放队列、随机与循环播放
- Radix 浅色 / 深色 / 跟随系统

未登录可浏览和播放 QQ 允许匿名收听的歌曲。左下角「登录 QQ 音乐」使用**手机 QQ**扫码；当前暂未适配微信扫码。会员与版权权限由 QQ 音乐服务决定。

## 构建

需要 Go 1.27.3+，无需 C 工具链。

```bash
go tool mygo dev     # 开发模式运行
go tool mygo build -platform windows/amd64  # 打包到 build/
go test ./internal/...                      # 运行测试
go tool mygo vet .
```

Windows 安装包与便携版在 `build/`。默认协议测试使用本地模拟服务。设置 `LIVE=1` 后运行 `go test ./internal/qqmusic -run TestLivePublicQQ -v`，验证匿名曲库、歌词及实际 CDN 音频。真人扫码、个人曲库写入和会员播放需真实账号验收。

`internal/qqmusic` 实现 musicu RPC、CGI、QQ 互联 HTTP 扫码/OAuth 和 QQ 数据转换，不启动或调用 Node 服务。

## 致谢

- [MyGo](https://github.com/egoist/mygo)：原生 UI 与应用框架
- [mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic)：原生界面与播放器
- [qq-music-api](https://github.com/Rain120/qq-music-api)：提供登陆、个人内容获取、歌曲播放等

本项目与 QQ 音乐官方无关，音乐版权归各权利人所有。

## 许可证

[AGPL-3.0](LICENSE)
