# Linux 桌面歌词：依赖、安装与限制

本文介绍 Lilt 在 Linux 下的桌面歌词运行依赖、安装步骤及兼容范围。包名与软件源示例核对于 **2026-11-10**；不同发行版及版本的软件包供应可能随时间变化。

目前已在 **AlmaLinux 10.2 x86_64、KDE Plasma Wayland、150% 显示缩放** 下完成人工验证：覆盖拖拽定位、锁定 / 解锁、任务栏与菜单避让，以及锁定后的鼠标穿透。

## 运行依赖

| 组件 | 用途 | 是否需要单独安装 |
| --- | --- | --- |
| GTK 3 / GDK | 原生窗口和鼠标输入 | Linux 图形界面的基础依赖；发行包之外的系统需自行提供 |
| GLib / GIO、Cairo、Wayland 客户端库 | 基础运行库、输入区域和桌面通信 | 通常随 GTK 3 或 layer-shell 一同安装 |
| **GTK 3 版 gtk-layer-shell**（提供 `libgtk-layer-shell.so.0`） | Wayland 面板定位、拖拽与层级控制 | **Wayland 下推荐安装**；完整使用本文的定位适配需同时具备该库及合成器协议支持 |
| 用户会话 D-Bus 与运行中的 Plasma Shell | 查询 KDE 任务栏所在屏幕、边缘和厚度 | KDE 桌面通常已提供；仅用于 KDE 任务栏区域避让 |
| Ayatana AppIndicator 或 AppIndicator 的 GTK 3 库 | 系统托盘图标和托盘解锁入口 | **可选**；无托盘时仍可从主窗口解锁歌词 |

**依赖说明：**

- **版本要求**：请务必安装 **GTK 3** 对应的 layer-shell 库，`gtk4-layer-shell` 无法替代。
- **仅需运行时**：运行预编译的 Lilt 不需要安装 Go、C 编译器或 `-dev` / `-devel` 开发包。
- **无需 WebKitGTK**：当前应用使用原生界面（`mygo.json` 中 `linux.webview` 为 `false`），不要求为桌面歌词另装 WebKitGTK。
- **无需 `qdbus` 命令**：KDE 面板读取直接通过 GIO / D-Bus 完成，**无需安装 `qdbus` 命令**。
- **范围说明**：本文仅列出与图形界面和桌面歌词相关的依赖，并非音频或其他可选功能的完整依赖清单。

## 安装依赖

请在终端中根据你当前使用的**发行版、系统版本及 CPU 架构**选择对应的示例执行，不需要把所有示例执行一遍。

### RPM 体系：Fedora、RHEL 兼容发行版与 openSUSE

以下发行版虽然均使用 RPM 包格式，但包名、包管理器和软件源互不通用。Fedora / RHEL 兼容发行版请使用下面的 DNF 示例，openSUSE 请使用 Zypper 示例，**切勿互相套用软件源或混装不同发行版的 RPM 包**。

#### Fedora

```bash
sudo dnf install gtk3 gtk-layer-shell
```

Fedora 官方仓库直接提供 `gtk-layer-shell`；Enterprise Linux 的包供应与 Fedora 不同，不能直接套用 Fedora 版本的 RPM。[Fedora 软件包页面](https://packages.fedoraproject.org/pkgs/gtk-layer-shell/gtk-layer-shell/)

#### AlmaLinux / Rocky Linux 等 Enterprise Linux 兼容发行版

可先查询当前已启用的软件源及可用包：

```bash
dnf repolist --enabled
dnf list --available gtk-layer-shell
```

以 AlmaLinux / Rocky Linux 9 为例，可先启用 EPEL 和 CRB，再执行安装：

```bash
sudo dnf install epel-release dnf-plugins-core
sudo dnf config-manager --set-enabled crb
sudo dnf install gtk3 gtk-layer-shell
```

RHEL 官方系统启用 EPEL 的方式可能有所不同，请按 [EPEL 官方快速入门](https://docs.fedoraproject.org/en-US/epel/getting-started/) 配置。上面的命令不代表所有 EL 版本的 EPEL 均已收录该包。

**AlmaLinux 10.2 x86_64 实测示例：** 本次验证时虽已启用 EPEL 和 CRB，但仍未在这些已启用的软件源中找到 `gtk-layer-shell`，因此使用了下面的第三方 COPR 构建：

```bash
sudo dnf install gtk3 epel-release dnf-plugins-core
sudo dnf copr enable yselkowitz/wlroots-epel epel-10-x86_64
sudo dnf install gtk-layer-shell
```

AlmaLinux 10 已默认启用 CRB；如果系统管理员曾手动关闭它，应先检查自身的软件源配置。[AlmaLinux CRB 说明](https://almalinux.org/blog/2025-09-08-enabling-crb-by-default-for-almalinux10/)

> **第三方源说明**：`yselkowitz/wlroots-epel` 是社区维护的第三方源，**不是 AlmaLinux 官方源或 EPEL 正式仓库**。是否信任并长期启用该源由用户自行决定；启用后也可能影响该源中其他同名包的安装和更新。`epel-10-x86_64` 仅适用于对应的 EL10 x86_64 构建，不应原样用于 EL9、Fedora 或 ARM64。若该源不可用，应查询匹配当前版本的其他来源，或按上游说明自行编译构建。[已核对的 EL10 构建目录](https://download.copr.fedorainfracloud.org/results/yselkowitz/wlroots-epel/epel-10-x86_64/)、[DNF COPR 插件说明](https://dnf-plugins-core.readthedocs.io/en/latest/copr.html)

#### openSUSE Tumbleweed / Leap

openSUSE 的 GTK 3 layer-shell 运行时包名为 `libgtk-layer-shell0`。请先检查当前版本已启用的软件源是否提供该包：

```bash
zypper repos -u
zypper search -s libgtk-layer-shell0
```

存在匹配的软件包时，安装示例为：

```bash
sudo zypper refresh
sudo zypper install libgtk-layer-shell0
```

Zypper 会自动解析所需的 GTK 3 运行库依赖，无需安装 `gtk-layer-shell-devel`。SUSE 的包索引列有该运行时子包。[SUSE layer-shell 包信息](https://packagehub.suse.com/packages/gtk-layer-shell/)

Tumbleweed 与 Leap 不同版本的仓库、包版本及支持架构可能不同。若找不到包，应查询匹配当前系统版本的 openSUSE 仓库；切勿把 Tumbleweed 源加到 Leap，不要套用前面的 EL10 COPR 示例，也不要因为查阅了 SUSE Package Hub 索引而在 openSUSE 上启用 SLE 的软件源。

> **注**：此处的“RPM 体系”仅指**系统依赖的安装方式**。安装 Lilt 本体请使用本项目的 Linux `.tar.gz` 和 `install.sh`；切勿将构建生成的 `.deb` 包直接交给 DNF 或 Zypper 安装。

### Debian / Ubuntu 系

Debian 13、Ubuntu 24.04 的安装示例：

```bash
sudo apt update
sudo apt install libgtk-3-0t64 libgtk-layer-shell0
```

Debian 12 等使用旧版 GTK 3 包名的系统：

```bash
sudo apt update
sudo apt install libgtk-3-0 libgtk-layer-shell0
```

`libgtk-layer-shell0` 会自动拉取所需的 GTK、GLib 和 Wayland 依赖；具体包名以系统所在版本为准。[Debian layer-shell 包](https://packages.debian.org/trixie/libgtk-layer-shell0)、[Debian 12 GTK 3 包](https://packages.debian.org/bookworm/libgtk-3-0)、[Ubuntu 24.04 GTK 3 包](https://packages.ubuntu.com/noble/libgtk-3-0t64)

Ubuntu 24.04 的 `libgtk-layer-shell0` 位于 `Universe` 仓库；若提示找不到候选包，请先检查 `Universe` 是否已启用，再更新软件包索引。[Ubuntu layer-shell 包](https://packages.ubuntu.com/en/noble/libgtk-layer-shell0)

### Arch 系

```bash
sudo pacman -Syu gtk3 gtk-layer-shell
```

Arch 的包位于官方 `Extra` 仓库；衍生发行版应使用各自的软件源，切勿直接混用其他发行版的仓库。[Arch layer-shell 包](https://archlinux.org/packages/extra/x86_64/gtk-layer-shell/)

### 可选：系统托盘依赖

若桌面没有显示托盘图标，可先检查桌面环境是否启用了系统托盘，并按需安装对应运行库：

| 系列 | 安装示例 |
| --- | --- |
| Fedora / 提供该包的 Red Hat 系仓库 | `sudo dnf install libayatana-appindicator-gtk3` |
| openSUSE / 提供该包的对应版本仓库 | `sudo zypper install libayatana-appindicator3-1` |
| Debian / Ubuntu | `sudo apt install libayatana-appindicator3-1` |
| Arch | `sudo pacman -Syu libappindicator` |

包名同样会随仓库版本变化（例如部分旧版 Arch 系仓库可能仍使用 `libappindicator-gtk3`）。注意：托盘库安装成功仅提供应用侧支持，不代表桌面环境一定自带托盘宿主。[Fedora 托盘库](https://packages.fedoraproject.org/pkgs/libayatana-appindicator/libayatana-appindicator-gtk3/)、[SUSE 托盘库](https://packagehub.suse.com/packages/libayatana-appindicator/)、[Debian 托盘库](https://packages.debian.org/trixie/libayatana-appindicator3-1)、[Arch 托盘库](https://archlinux.org/packages/extra/x86_64/libappindicator/)

## 安装后检查与使用

安装完系统依赖后，**请从托盘完全退出 Lilt，再重新启动**。仅关闭主窗口可能仍留有后台进程；进程首次启动探测 layer-shell 后，不会在运行期间自动热加载新安装的动态库。

常见 glibc 发行版可检查动态库缓存：

```bash
ldconfig -p | grep 'libgtk-layer-shell.so.0'
```

正常情况下应看到 `libgtk-layer-shell.so.0`。该检查仅证明系统动态库缓存中存在该库，不能证明当前合成器支持对应协议，也不能保证容器内的进程能够加载宿主机的动态库。

检查当前桌面会话信息：

```bash
printf '会话类型：%s\n桌面：%s\nWayland 显示：%s\nGTK 后端覆盖：%s\n' \
  "$XDG_SESSION_TYPE" "$XDG_CURRENT_DESKTOP" "$WAYLAND_DISPLAY" "$GDK_BACKEND"
```

Wayland 会话下通常可看到 `wayland` 和非空的 `WAYLAND_DISPLAY`。注意：若设置了 `GDK_BACKEND=x11`，GTK 应用会强制选择 X11 路径，因此不能仅凭会话名称认定应用正运行在原生 Wayland 下。

将下载的 `install.sh` 和匹配架构的 `.tar.gz` 放在同一目录后，可在该目录下运行：

```bash
sh install.sh
```

脚本会优先查找同目录下版本与架构匹配的本地压缩包，并安装到当前用户的 `~/.local/lilt.app`，无需 `sudo`；若找不到匹配的本地压缩包，脚本会尝试联网下载（若要确保安装指定的本地修复包，可显式传入压缩包路径）。应用安装脚本不会自动配置第三方源或安装本文列出的可选依赖；使用 `.deb` 安装时同样不能假定系统已安装 layer-shell 这一可选库。

开启桌面歌词后，将鼠标移到**未锁定**的歌词区域即可显示工具栏并拖拽位置；点击**锁定**后，鼠标事件将穿透到下方窗口。解锁入口有两个：

- **主窗口底部播放栏**：右键点击「桌面歌词」按钮（无系统托盘时仍可使用此入口）。
- **系统托盘**：右键点击应用图标，选择「解锁桌面歌词」。

## 兼容范围

| 功能 | 代码适用范围 | 缺少条件时的行为 |
| --- | --- | --- |
| Wayland 精确定位与拖拽 | 已安装 GTK 3 layer-shell 库，且合成器支持 layer-shell | 退回普通窗口，出现位置由合成器决定，无法保证恢复原位置 |
| 分数缩放下的屏幕逻辑边界修正 | 满足上述 layer-shell 路径，且提供 xdg-output v2 或更高版本 | 沿用 GTK 提供的屏幕边界，仍可能遇到定位偏差 |
| 歌词与弹出菜单的层级 | layer-shell 路径下使用 TOP 层 | 无 layer-shell 时沿用普通窗口行为，具体顺序取决于桌面环境 |
| 锁定后的鼠标穿透 | Linux GTK 3 的 X11 / Wayland 后端 | 由框架保留锁定输入状态，在窗口重新映射（remap）时重新应用 |
| 任务栏区域避让 | **KDE Plasma Wayland**，且可访问 Plasma 的会话 D-Bus 接口 | 使用原有工作区边界，无法保证避让任务栏 |

layer-shell 上游列出的支持环境包括 KDE Plasma Wayland 和 Sway 等 wlroots 合成器；**GNOME Wayland 不提供该协议**。安装动态库不能替合成器增加协议支持。此外，其他桌面的代码兼容范围不等于 Lilt 已在这些桌面完成实机验收。[上游支持环境说明](https://github.com/wmww/gtk-layer-shell#supported-desktops)

## 限制与局限

1. **人工验证范围有限。** 本轮实际验收环境为 AlmaLinux 10.2、KDE Plasma Wayland、x86_64、150% 缩放。Fedora、openSUSE、Debian、Ubuntu、Arch 的安装示例已核对包名和来源，但未逐一实机安装并验收；X11、其他 Wayland 合成器、ARM64 及多显示器组合仍需对应环境验证。Windows / macOS 的构建通过不代表 Linux 修复也在多环境下完成了实机验收。
2. **任务栏避让并非通用 Wayland 能力。** 当前 KDE 适配要求 `XDG_CURRENT_DESKTOP` 包含 `KDE`，并能访问 `org.kde.plasmashell`。若查询失败、会话接口受限或使用其他桌面，将保留原有边界；这不会阻止应用启动，但歌词可能会覆盖任务栏。
3. **面板几何采用近似边缘区域。** 程序根据 Plasma 面板所在屏幕、边缘和厚度，直接预留整条对应屏幕边缘，不会按短面板的实际长度逐像素避让。自动隐藏面板同样会预留区域（不跟踪显示动画和实时可见性）；浮动面板间距、特殊布局或第三方面板不保证精确匹配。面板配置约每两秒查询一次，修改配置后可能短暂使用旧边界。
4. **Wayland 拖动限制在当前输出内。** layer-shell 拖拽使用窗口内的指针坐标，目前不能保证像 X11 普通窗口那样直接拖过显示器边界。显示器变化时会尝试把窗口限制到可达区域，但混合缩放、热插拔及多任务栏组合尚未完成全面人工验证。
5. **置顶不等于覆盖所有窗口。** 歌词使用 `TOP` 层，以便让弹出菜单位于其上方。合成器仍决定最终层级，活动的全屏窗口可能会盖住歌词；本轮适配不承诺在所有全屏应用上方显示。
6. **坐标修正不等于完整的分数缩放渲染。** 本修复仅解决 GTK 屏幕边界与合成器逻辑坐标不一致的问题，不会使 GTK 3 自动具备原生分数缩放渲染能力，也不承诺消除所有字体模糊或缩放差异。
7. **库和桌面接口必须对运行进程可见。** 容器、Flatpak、Snap 等隔离环境可能看不到宿主机的动态库或 Plasma D-Bus 接口；本文未提供这些环境的打包与权限适配。请勿通过 `sudo` 启动桌面应用来替代正常的用户会话。
8. **安装包与系统依赖分别维护。** 当前安装包不捆绑 GTK、`gtk-layer-shell` 或完整桌面环境。第三方软件源的可用性、版本兼容和后续更新由各仓库维护；无匹配包时不能用其他 EL 大版本或其他 CPU 架构的 RPM 替代。

## 故障反馈

出现定位、层级或鼠标穿透问题时，提交反馈请尽量提供以下信息：

- **基础环境**：发行版及版本、桌面 / 合成器及版本、会话类型（X11 或 Wayland）、CPU 架构。
- **显示与依赖**：显示缩放比例、显示器数量、`gtk-layer-shell` 包版本。
- **复现条件**：问题发生在锁定还是解锁状态？是否在重新开启歌词、重启应用或改变显示设置后出现？（这有助于区分窗口生命周期问题和坐标问题）。

维护者可使用隔离配置的离线探针复查，不会影响正常应用的数据：

```bash
WAYLAND_DEBUG=1 go run ./cmd/desktop-lyrics-probe \
  -output /tmp/lilt-lyrics-check -duration 21s -check \
  2>/tmp/lilt-lyrics-wayland.log
```

此命令需要项目源码和对应要求的 Go 版本，执行后会打开验证窗口。锁定阶段应向对应的歌词 layer surface 提交空输入区域，解锁后恢复输入；尤其应检查 `restored-locked` 阶段，避免锁定后重新开启歌词时再次拦截下方窗口。`native-report.json` 可用于检查任务栏扣除后的工作区及窗口边界；Wayland 协议日志仅能验证输入区域设置，不能替代各桌面环境下的人工点击测试。
