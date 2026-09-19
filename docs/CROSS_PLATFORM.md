# OneProxy 跨平台状态

## 当前支持范围

OneProxy 由 Go 核心、Qt 托盘和 sing-box 三部分组成。项目已经建立跨平台编译基础，但目前只有 Windows 经过实际运行验证并提供正式安装包。

| 平台 | CI 编译 | 实际运行验证 | 安装包/发行物 |
|------|---------|--------------|---------------|
| Windows 2022 | ✅ | ✅ | ✅ |
| macOS 15 | ✅ | ❌ | CI zip（ad-hoc 临时签名，未公证） |
| Ubuntu 24.04 | ✅ | ❌ | ❌ |

macOS 和 Linux 的 CI 通过仅表示对应平台的构建检查通过，不表示代理进程管理、DNS 刷新、托盘行为或系统集成已经可用。macOS CI 还会生成包含 Qt 运行库、Go 共享库、sing-box 和规则集的 `.app` zip，完成 ad-hoc 临时签名，并校验解压后的签名完整性及 Go 共享库加载。这不代表 Apple 开发者身份认证或公证。

## 构建基线

- Go 1.26.x
- Qt 6.8.3
- CMake 3.16+
- Windows：MSVC 2022
- macOS：Apple Clang
- Linux：GCC

三平台 CI 都会运行 `go test ./...` 和 `go vet ./...`，并构建：

- `cmd/oneproxy` CLI
- `cmd/oneproxy-dll` 平台共享库（DLL、dylib 或 SO）
- `trayapp` Qt 托盘程序

Go 共享库使用 `buildmode=c-shared`，因此必须在目标平台原生构建并启用 CGO。

## Windows

Windows 是当前唯一受支持的平台。以下能力经过实际验证：

- Qt 系统托盘
- sing-box 启动、停止和健康检查
- Windows Job Object 子进程管理
- `ipconfig /flushdns` DNS 缓存刷新
- 开机启动和安装包升级
- Inno Setup 安装包与 GitHub Release 发布

Windows 发布任务只有在 Windows、macOS 和 Linux 三个平台的编译检查全部通过后才会运行。

## macOS

当前状态为“CI 构建、ad-hoc 签名和 `.app` 打包验证”。项目不提供 DMG、Developer ID 签名或 Apple 公证，也没有完成用户桌面上的运行验收。

### 下载和首次打开

CI 只提供 `OneProxy-macos-universal` 产物，同一个应用支持 Intel 和 M 系列 Mac。解压 GitHub artifact ZIP，再解压其中的应用 ZIP，将 `oneproxy-tray.app` 拖入“应用程序”。双击的是整个 `.app`，不是 `Contents/MacOS` 内部的可执行文件。

此版本未公证，macOS 可能阻止首次打开。先尝试打开一次，再到“系统设置 → 隐私与安全性”选择“仍要打开”。如果提示“已损坏”，先检查包的完整性：

```bash
codesign --verify --deep --strict --verbose=2 /Applications/oneproxy-tray.app
```

若校验失败，请重新下载新版，不要直接移除系统隔离标记。若校验通过、确认应用来自本仓库的构建，但仍受下载隔离拦截，可仅移除此应用的隔离标记后再打开：

```bash
xattr -dr com.apple.quarantine /Applications/oneproxy-tray.app
open /Applications/oneproxy-tray.app
```

这不关闭系统 Gatekeeper，也不使应用获得 Apple 公证。CI 的签名校验通过不能代替真实下载后的 Gatekeeper 验证。

### 从源码编译和打包

需要 macOS、Xcode 命令行工具、Go 1.26+、CMake 3.21+、Python 3 和 Qt 6.8.3。若 CMake 安装在 Python 虚拟环境内，请先激活环境。Qt 安装目录必须包含 `bin/macdeployqt`，并提供目标架构的库；官方双架构 Qt 可用于交叉编译及 universal 包。不依赖 Homebrew。

```bash
# 在源码目录执行；--qt-root 按实际 Qt 安装位置填写
bash scripts/build-macos.sh --arch x86_64 --qt-root "$HOME/Qt"
bash scripts/build-macos.sh --arch arm64 --qt-root "$HOME/Qt"
bash scripts/build-macos.sh --arch universal --qt-root "$HOME/Qt"
```

`--arch` 默认使用当前主机架构；`--qt-root` 默认读取 `QT_ROOT_DIR`，未设置时使用 `$HOME/Qt`。标准 aqt 安装位置可能是 `$HOME/Qt/6.8.3/macos`，需显式传入该路径。`--skip-tests` 可跳过 Go 测试、vet 和托盘测试，但保留打包、架构、签名及可在本机运行的组件检查。

脚本从自身位置定位仓库，不依赖终端当前目录。它构建 CLI 和共享库、编译托盘、下载并校验固定版本 sing-box、生成规则集、部署 Qt，最后进行 ad-hoc 签名和 ZIP 解压回验。只执行测试与组件检查，不启动代理或修改系统 DNS。

| 产物 | 路径（`<arch>` 为参数值） |
|------|-------------------------|
| CLI 和 Go 动态库 | `dist/macos-<arch>/` |
| 完整应用 | `trayapp/build-macos-<arch>/oneproxy-tray.app` |
| 分发压缩包 | `dist/OneProxy-macos-<arch>.zip` |

交叉编译时用主机版本的 sing-box 生成规则集，包内放入目标版本；universal 包合并两种架构的 CLI、动态库和 sing-box。Go 测试在主机架构上执行，单架构交叉构建不执行目标托盘及组件运行检查，仍检查所有包内 Mach-O 文件是否包含目标架构。完整桌面行为需在对应架构的 Mac 上验证。

后续正式支持前至少需要验证：

- sing-box 子进程生命周期与睡眠唤醒行为
- `dscacheutil` 和 `mDNSResponder` DNS 刷新及管理员权限方案
- 菜单栏图标、配置文件打开和路由模式持久化
- `.app` Bundle、Qt 部署和动态库加载路径
- Apple Silicon 与 Intel 架构兼容性

## Linux

当前状态为“仅保证 Ubuntu 24.04 CI 编译”。项目不提供发行版安装包，也没有验证实际桌面运行。

后续正式支持前至少需要验证：

- sing-box 子进程生命周期
- `resolvectl` 等 DNS 刷新命令和权限
- X11、Wayland 与不同桌面环境下的托盘支持
- Qt 和共享库部署方式
- 不同 Linux 发行版的兼容性

## 开发边界

当前跨平台基础工作的目标是隔离 Windows 专属代码并保持三平台可编译，不包括：

- 在 macOS 或 Linux 上启动真实代理进行运行测试
- 修改系统 DNS 或安装提权 helper
- macOS Developer ID 签名、公证和 DMG
- Linux AppImage、Flatpak 或发行版软件包
- macOS/Linux Release 附件

在完成目标平台真机验证前，文档和界面不应宣称 macOS 或 Linux 已受支持。
