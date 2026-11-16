# 专辑版权信息与平台开关

桌面端的单曲、EP 和专辑详情页会在歌曲列表下方显示 QQ 音乐返回的唱片公司或版权声明。该功能通过应用层新增的编译期常量 `albumCreditsEnabled` 控制，并非用户设置项。

## 平台范围

| 平台 | 开关值 | 定义文件 |
| --- | --- | --- |
| Windows、macOS、Linux | `true` | [album_credits_desktop.go](../internal/app/album_credits_desktop.go) |
| Android、iOS、Web 及其他平台 | `false` | [album_credits_other.go](../internal/app/album_credits_other.go) |

上述两个文件使用互补的 Go 构建标签，编译时只会选入其中一个。桌面端的构建条件如下：

```go
windows || (darwin && !ios) || (linux && !android)
```

在 Go 的构建标签中，Android 同样匹配 `linux`，iOS 同样匹配 `darwin`，因此需要在条件中显式排除。

## 与音频开关的关系

这里沿用了[桌面音频开关](desktop-audio.md)的组织方式，但未直接复用 `desktopAudioFeatures`。音频开关位于 MyGo 的 `audio` 包内部，包含音频后端的架构限制；版权展示则由应用层独立控制，不附加这些架构限制，也不会影响音频功能。

## 数据与显示规则

- **字段回退**：优先读取 `company`，无有效内容时回退至 `company_new.name`，并保存至 `AlbumProfile.Company`。
- **无效值过滤**：去除首尾空白后，内容为空或等于「未确定」「未確定」时视为缺失；当两个字段均无效时，不创建版权页脚及其间距。
- **原文保留**：「独立发行」等有效内容正常显示，完整保留原文（包括年份、曲号、版权符号、授权关系与换行），不添加「唱片公司」前缀。
- **排版样式**：桌面端采用次要文字颜色，字号为 12.5 个逻辑单位、1.5 倍行高，最大宽度为 760 个逻辑单位，随窗口宽度自然换行以完整显示内容。

数据处理见 [catalog.go](../internal/qqmusic/catalog.go)，显示入口见 [ui_album.go](../internal/app/ui_album.go)。平台开关仅控制界面显示，不影响接口字段的读取。

## 后续扩展

- 若需为其他平台启用该功能，应先适配对应平台的布局，再调整构建条件；必要时可增加独立的平台策略文件。
- 所有平台文件的构建条件必须保持互斥且覆盖所有目标平台，确保每个编译目标下仅定义一次 `albumCreditsEnabled`。
- 调整版权展示逻辑或平台开关时，无需修改音频开关。

## 回归测试

可通过以下命令运行相关回归测试：

```bash
CGO_ENABLED=0 go test ./internal/qqmusic ./internal/app -run 'TestAlbum(Company|Credits)' -count=1
```
