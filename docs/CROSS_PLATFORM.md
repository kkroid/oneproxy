# OneProxy 跨平台状态

## 当前支持范围

OneProxy 由 Go 核心、Qt 托盘和 sing-box 三部分组成。项目已经建立跨平台编译基础，但目前只有 Windows 经过实际运行验证并提供安装包。

| 平台 | CI 编译 | 实际运行验证 | 安装包/发行物 |
|------|---------|--------------|---------------|
| Windows 2022 | ✅ | ✅ | ✅ |
| macOS 15 | ✅ | ❌ | ❌ |
| Ubuntu 24.04 | ✅ | ❌ | ❌ |

macOS 和 Linux 的 CI 通过仅表示 Go CLI、Go 共享库和 Qt 托盘可以在对应原生环境完成编译，不表示代理进程管理、DNS 刷新、托盘行为或系统集成已经可用。

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

当前状态为“仅保证 CI 编译”。项目不提供 `.app`、DMG、签名或公证，也没有在真实 macOS 环境中验证运行行为。

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
- macOS 签名、公证和 DMG
- Linux AppImage、Flatpak 或发行版软件包
- macOS/Linux Release 附件

在完成目标平台真机验证前，文档和界面不应宣称 macOS 或 Linux 已受支持。
