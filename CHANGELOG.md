# 更新日志

## 0.4.3

- 支持搜索自己歌单中的歌曲
- 修复 Linux Wayland 桌面歌词的定位、层级及锁定后鼠标穿透问题
- 关闭应用时遵循播放淡入淡出设置
- 优化 Windows 任务栏歌词

### 下载指南

| 处理器架构 | Windows | macOS | Linux |
| :--- | :--- | :--- | :--- |
| amd64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.3/lilt-setup-0.4.3-amd64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.3/lilt-0.4.3-amd64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.3/lilt_0.4.3_amd64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.3/lilt-0.4.3-linux-amd64.tar.gz) |
| arm64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.3/lilt-setup-0.4.3-arm64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.3/lilt-0.4.3-arm64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.3/lilt_0.4.3_arm64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.3/lilt-0.4.3-linux-arm64.tar.gz) |

> Linux：Ubuntu / Debian 等支持 deb 的发行版选 `.deb`，其他发行版选 `.tar.gz`。

## 0.4.2

- 正在播放页面支持调整歌词字号
- 个人资料显示 QQ 音乐会员到期时间
- 改进 QQ 与微信登录凭证续期
- 修复桌面端跨采样率手动切歌时旧歌曲声音残留，并平滑关闭旧音频输出
- 打磨多处细节

### 下载指南

| 处理器架构 | Windows | macOS | Linux |
| :--- | :--- | :--- | :--- |
| amd64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.2/lilt-setup-0.4.2-amd64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.2/lilt-0.4.2-amd64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.2/lilt_0.4.2_amd64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.2/lilt-0.4.2-linux-amd64.tar.gz) |
| arm64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.2/lilt-setup-0.4.2-arm64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.2/lilt-0.4.2-arm64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.2/lilt_0.4.2_arm64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.2/lilt-0.4.2-linux-arm64.tar.gz) |

> Linux：Ubuntu / Debian 等支持 deb 的发行版选 `.deb`，其他发行版选 `.tar.gz`。

## 0.4.1

- 降低音频播放的内存占用，未写入歌曲缓存的音频优先使用临时文件，播放结束后自动清理
- 优化封面加载与复用，按需生成主题色和模糊背景，减少重复下载、解码及内存占用
- 降低正在播放页面及展开、收起动画的渲染内存占用，复用裁剪缓冲并及时释放不再使用的 GPU 资源
- 升级 MyGo 至 v0.8.3，合入上游内存管理与渲染优化

### 下载指南

| 处理器架构 | Windows | macOS | Linux |
| :--- | :--- | :--- | :--- |
| amd64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.1/lilt-setup-0.4.1-amd64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.1/lilt-0.4.1-amd64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.1/lilt_0.4.1_amd64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.1/lilt-0.4.1-linux-amd64.tar.gz) |
| arm64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.1/lilt-setup-0.4.1-arm64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.1/lilt-0.4.1-arm64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.1/lilt_0.4.1_arm64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.1/lilt-0.4.1-linux-arm64.tar.gz) |

> Linux：Ubuntu / Debian 等支持 deb 的发行版选 `.deb`，其他发行版选 `.tar.gz`。

## 0.4.0

- Linux 新增实验性桌面歌词(仅部分桌面环境测试)
- 桌面歌词支持自定义颜色和透明度
- 待播清单改为浮动面板
- 新增播放操作淡入淡出开关
- 优化播放和歌词显示的资源占用
- 打磨优化部分细节及体验
- 优化 Windows 任务栏歌词
- 调整应用图标样式和 macOS 图标尺寸，修复 macOS 刘海播放器展开时出现白边的问题
- 桌面端音频输出支持跟随音源采样率，设备冷启动、切换采样率或唤醒时进行短暂静音预热
- 正在播放页面新增从播放器封面展开的过渡动画，降低过渡时的内存占用
- 修复正在播放页面的播放时间可能停止更新的问题
- 修复部分音频后端在歌曲自然结束后可能无法继续播放的问题

### 下载指南

| 处理器架构 | Windows | macOS | Linux |
| :--- | :--- | :--- | :--- |
| amd64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.0/lilt-setup-0.4.0-amd64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.0/lilt-0.4.0-amd64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.0/lilt_0.4.0_amd64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.0/lilt-0.4.0-linux-amd64.tar.gz) |
| arm64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.0/lilt-setup-0.4.0-arm64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.0/lilt-0.4.0-arm64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.0/lilt_0.4.0_arm64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.4.0/lilt-0.4.0-linux-arm64.tar.gz) |

> Linux：Ubuntu / Debian 等支持 deb 的发行版选 `.deb`，其他发行版选 `.tar.gz`。

## 0.3.0

- Windows 和 macOS 新增实验性桌面歌词，独立显示逐字高亮的当前歌词，支持拖动、调整宽度和字号、锁定位置及悬停播放控制
- Windows 新增实验性任务栏歌词，在空间允许的横向主任务栏中显示当前歌词与封面；点击封面可打开播放控制浮层，进行上一首、播放/暂停、下一首和进度跳转；灵感来源[TaskbarLyrics](https://github.com/ANYNC/TaskbarLyrics)
- 设置中可统一选择桌面或任务栏歌词，并可从播放栏或托盘快速开关；关闭歌词后会保留当前模式和桌面歌词偏好
- 歌曲菜单新增「下一首播放」，可将歌曲直接插入当前播放项之后

### 下载指南

| 处理器架构 | Windows | macOS | Linux |
| :--- | :--- | :--- | :--- |
| amd64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.3.0/lilt-setup-0.3.0-amd64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.3.0/lilt-0.3.0-amd64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.3.0/lilt_0.3.0_amd64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.3.0/lilt-0.3.0-linux-amd64.tar.gz) |
| arm64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.3.0/lilt-setup-0.3.0-arm64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.3.0/lilt-0.3.0-arm64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.3.0/lilt_0.3.0_arm64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.3.0/lilt-0.3.0-linux-arm64.tar.gz) |

> Linux：Ubuntu / Debian 等支持 deb 的发行版选 `.deb`，其他发行版选 `.tar.gz`。

## 0.2.2

- 逐字歌词完全适配 QQ 音乐官方 QRC 逐字时间轴，暂停或回跳后高亮仍紧跟播放位置
- 音量平衡从单一开关扩展为关闭、安静、标准、高音量四档，旧设置的开启对应「标准」挡位
- 修复主窗口关闭到托盘后，再次点击应用快捷方式会新开一个应用的问题
- 音量控件上鼠标滚轮改为每次滚动 5%，触控板滑动保持连续变化

### 下载指南

| 处理器架构 | Windows | macOS | Linux |
| :--- | :--- | :--- | :--- |
| amd64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.2/lilt-setup-0.2.2-amd64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.2/lilt-0.2.2-amd64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.2/lilt_0.2.2_amd64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.2/lilt-0.2.2-linux-amd64.tar.gz) |
| arm64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.2/lilt-setup-0.2.2-arm64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.2/lilt-0.2.2-arm64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.2/lilt_0.2.2_arm64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.2/lilt-0.2.2-linux-arm64.tar.gz) |

> Linux：Ubuntu / Debian 等支持 deb 的发行版选 `.deb`，其他发行版选 `.tar.gz`。

## 0.2.1

- 以补丁形式修复MyGo Windows 层在多个缩放比例不同的显示器之间恢复窗口时，窗口位置和尺寸可能逐次漂移的问题
- 优化歌单与专辑完整简介浮层的滚动条间距，避免内容与滚动条过于贴近
- 升级上游 MyGo 0.7.4，改善窗口首次显示时的白屏闪烁、原生菜单交互及 macOS 闲置功耗

### 下载指南

| 处理器架构 | Windows | macOS | Linux |
| :--- | :--- | :--- | :--- |
| amd64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.1/lilt-setup-0.2.1-amd64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.1/lilt-0.2.1-amd64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.1/lilt_0.2.1_amd64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.1/lilt-0.2.1-linux-amd64.tar.gz) |
| arm64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.1/lilt-setup-0.2.1-arm64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.1/lilt-0.2.1-arm64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.1/lilt_0.2.1_arm64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.1/lilt-0.2.1-linux-arm64.tar.gz) |

> Linux：Ubuntu / Debian 等支持 deb 的发行版选 `.deb`，其他发行版选 `.tar.gz`。

## 0.2.0

- 首页新增更多样式歌曲推荐，对齐官方，并优化首页布局
- Windows 和 Linux 的设置页改为在主窗口中打开，macOS依然对齐系统设计
- 优化底部播放栏的悬停、按压和键盘聚焦反馈，音量面板展开时保持按钮高亮
- 合并上游，优化封面缓存，减少返回已浏览页面时的重复加载，复用播放页仍持有的封面

### 下载指南

| 处理器架构 | Windows | macOS | Linux |
| :--- | :--- | :--- | :--- |
| amd64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.0/lilt-setup-0.2.0-amd64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.0/lilt-0.2.0-amd64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.0/lilt_0.2.0_amd64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.0/lilt-0.2.0-linux-amd64.tar.gz) |
| arm64 | [安装包 .exe](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.0/lilt-setup-0.2.0-arm64.exe) | [安装包 .dmg](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.0/lilt-0.2.0-arm64.dmg) | [.deb](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.0/lilt_0.2.0_arm64.deb) · [.tar.gz](https://github.com/Amamiya23/mygo-qyin/releases/download/v0.2.0/lilt-0.2.0-linux-arm64.tar.gz) |

> Linux：Ubuntu / Debian 等支持 deb 的发行版选 `.deb`，其他发行版选 `.tar.gz`。

## 0.1.8

- 打磨多处细节
- 新增音量平衡，设置中默认开启，减小不同歌曲之间的音量差异
- 进度条新增「波浪」特效，可在设置中切换标准、烟雾和波浪三种样式
- 支持更改歌曲缓存目录，重启后自动迁移已有缓存
- 改善歌曲、歌手和专辑的搜索稳定性，接口受限时自动切换备用来源，并优化分页和失败重试
- 优化搜索联想，快速输入或切换关键词时及时取消旧请求，避免显示过期建议；搜索历史弹窗与搜索框对齐
- 当前歌曲滚出列表后，可通过定位按钮快速返回；在歌单中再次点击当前歌曲可暂停或继续播放
- 歌单和专辑简介过长时，可通过悬停或键盘聚焦查看完整内容
- 优化播放页、律动和选歌视图中的按钮反馈，悬停时图标更清晰

## 0.1.7

- 修复「今日为你推荐」重复精选歌单的问题，改为独立读取账号首页推荐
- 修正排行榜内容类型筛选，排除无法按歌曲加载的专用榜单，避免打开后显示空列表
- 从歌单返回「为你推荐」或排行榜时恢复原来的滚动位置，前进和重新访问页面也会保留各自的位置
- macOS 刘海播放器新增同步歌词：虚拟刘海收起时显示当前歌词，展开播放时在歌名下显示歌词
- 修复刘海播放器在暂停后拖动播放进度时，歌词未及时刷新的问题

## 0.1.6

- 启用应用内自动更新，支持检查新版本、查看更新说明、下载安装和重启生效；旧版用户需手动安装一次本版本
- 新增微信扫码登录，可在登录窗口切换 QQ / 微信
- 我的主页显示会员状态，支持查询失败后重试
- 「我喜欢」支持单曲 / 专辑切换，专辑页可收藏与取消收藏
- 「我喜欢」单曲与专辑共用大封面页头，切换时保持布局稳定，分别记住滚动位置，并增加标签切换动效
- 侧栏新增「已购音乐」，登录后可分别浏览已购单曲与数字专辑，支持分页、刷新和专辑跳转
- 修复收藏歌单名称显示及取消收藏功能
- 修复登录后歌手页的专辑列表加载失败
- 修复部分歌曲的翻译歌词缺失，以及歌曲评论无法加载和翻页的问题
- 正在播放的标签保留在右侧内容区上方，在切换页面和评论数量变化时保持位置稳定，大窗口中的封面与内容居中显示
- 正在播放的封面、歌名和进度条左侧对齐，调整标签间距并缩短选歌封面墙顶部的模糊过渡
- 修复 Windows 右键菜单宽度计算不足，导致歌手名、专辑名等文字右侧被裁切的问题
- Windows 侧边栏默认使用亚克力，保留已保存的 Mica 选择；macOS 和 Linux 不显示 Windows 材质设置
- 关闭主窗口的设置按平台显示后台运行或托盘选项，Windows/Linux 无法创建托盘时关闭窗口直接退出

## 0.1.5

- 修复登录后搜索歌曲、歌手和专辑返回空结果的问题
- 搜索接口拒绝或返回异常格式时正确报告失败，避免误显示为没有结果
- 项目更名为 Lilt，统一应用名称、安装包、便携包和网站品牌
- 首次启动自动迁移旧 Q 音版的设置、播放队列和缓存，沿用原应用标识以保留登录态
- Windows 侧边栏支持在 Mica / 亚克力间切换，亚克力使用 40% 磨砂遮罩与低对比控件
- 已喜欢歌曲的爱心及点击动效统一使用 Radix 红色，保留绿色主题和未收藏时的空心样式

## QQ 音乐 0.1.4

- 修复歌单歌曲行、侧栏迷你播放器和正在播放详情等位置的封面缺失
- 封面请求使用 QQ CDN 支持的尺寸档位，避免任意尺寸地址返回 404

## QQ 音乐 0.1.3

- 修复 Windows 已配置系统代理时，客户端仍绕过代理直连而导致二维码获取中断的问题
- 登录、曲库、封面与音频下载共用系统代理选择，并遵循环境变量优先级和代理绕过规则
- 取码失败保留具体原因，支持失败后再次刷新并显示新二维码

## QQ 音乐 0.1.2

- 修复登录响应的有效期字段以数字字符串返回时被错误拒绝的问题
- 区分 QQ 音乐服务拒绝码、响应解析、缺失凭据及网络连接错误，便于定位实际失败阶段

## QQ 音乐 0.1.1

- 修复扫码确认失败后错误地址撑出对话框的问题，按登录步骤显示简短提示
- 修复已经扫描时出现错误却无法刷新二维码的问题
- 授权 GET 在 TLS 建连中断、请求尚未发出时重新连接一次
