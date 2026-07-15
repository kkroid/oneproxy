# OneProxy 跨平台可行性报告

## 架构分析

OneProxy 由两个主要组件组成：

| 组件 | 语言 | 依赖 | 跨平台性 |
|------|------|------|----------|
| **oneproxy.dll** | Go 1.21+ | Go 标准库 + sing-box | ✅ 完全跨平台 |
| **oneproxy-tray.exe** | C++ Qt6 | Qt6 Widgets | ✅ Qt 跨平台框架 |

---

## Windows（当前平台）✅

**状态：** 已完成，生产就绪

**工具链：**
- Go 1.21+ (cross-compile ready)
- MSVC 2022 或 MinGW-w64
- Qt 6.8 (MSVC/MinGW)

**特性支持：**
- ✅ 系统托盘（QSystemTrayIcon）
- ✅ DNS 刷新（`ipconfig /flushdns`）
- ✅ 无控制台窗口（`SysProcAttr{HideWindow: true}`）
- ✅ sing-box 隐藏启动

**已知问题：** 无

---

## macOS（理论可行）🟡

**可行性：** 高（95%）

### Go 核心 (oneproxy.dll → oneproxy.dylib)

**修改量：** 最小
- ✅ Go `buildmode=c-shared` 原生支持 macOS
- ✅ `exec.Command` 跨平台
- ⚠️ `syscall.SysProcAttr{HideWindow: true}` Windows 专用 → macOS 不需要（无控制台）
- ⚠️ DNS 刷新命令已支持（`dscacheutil -flushcache` + `killall -HUP mDNSResponder`）

**构建：**
```bash
go build -buildmode=c-shared -o oneproxy.dylib ./cmd/oneproxy-dll
```

### Qt 托盘 (oneproxy-tray.app)

**修改量：** 中等
- ✅ Qt6 原生支持 macOS
- ✅ `QSystemTrayIcon` 在 macOS 上为菜单栏图标（标准行为）
- ⚠️ 图标格式：需要 PNG（当前为 ICO）或使用 Qt 资源系统
- ⚠️ macOS 应用打包：需要 `.app` bundle + `Info.plist`

**工具链：**
```bash
brew install qt@6
cmake -DCMAKE_PREFIX_PATH=$(brew --prefix qt@6) ..
make
```

**打包 .app：**
```bash
macdeployqt oneproxy-tray.app -dmg
```

### sing-box

- ✅ sing-box 官方提供 macOS 二进制（darwin-amd64, darwin-arm64）

### 估计工作量

| 任务 | 时间 |
|------|------|
| Go DLL 适配（移除 Windows 特定代码） | 30 分钟 |
| Qt 图标格式转换（ICO → PNG/资源文件） | 1 小时 |
| CMakeLists.txt macOS 适配 | 1 小时 |
| .app bundle 打包脚本 | 2 小时 |
| 测试 + 文档 | 3 小时 |
| **总计** | **~1 天** |

---

## Linux（理论可行）🟡

**可行性：** 中（70%）

### Go 核心 (oneproxy.so)

**修改量：** 最小
- ✅ Go `buildmode=c-shared` 原生支持 Linux
- ⚠️ DNS 刷新命令已支持（`systemd-resolve --flush-caches` / `resolvectl flush-caches`）
- ⚠️ 需要 root 权限或 `CAP_NET_ADMIN` 才能刷新 DNS

**构建：**
```bash
go build -buildmode=c-shared -o oneproxy.so ./cmd/oneproxy-dll
```

### Qt 托盘 (oneproxy-tray)

**修改量：** 较大
- ✅ Qt6 原生支持 Linux
- ⚠️ `QSystemTrayIcon` 在 Linux 上依赖桌面环境：
  - GNOME：需要 [AppIndicator](https://github.com/ubuntu/gnome-shell-extension-appindicator) 扩展
  - KDE Plasma：原生支持
  - XFCE/LXDE：原生支持
- ⚠️ Wayland 会话下托盘支持不完整（部分桌面环境）
- ⚠️ 图标：需要 PNG/SVG（freedesktop 标准）

**依赖：**
```bash
# Ubuntu/Debian
sudo apt install qt6-base-dev libqt6widgets6

# Arch
sudo pacman -S qt6-base

# Fedora
sudo dnf install qt6-qtbase-devel
```

### sing-box

- ✅ sing-box 官方提供 Linux 二进制（多架构）

### 主要挑战

1. **托盘图标不统一：** GNOME 需要额外扩展，Wayland 支持不完整
2. **权限管理：** DNS 刷新可能需要 `sudo` 或 `polkit` 规则
3. **发行版差异：** 不同发行版的 Qt 版本、依赖路径差异大

### 估计工作量

| 任务 | 时间 |
|------|------|
| Go 核心适配 | 30 分钟 |
| Qt 图标 + 依赖适配 | 2 小时 |
| 处理 AppIndicator / GNOME 托盘问题 | 4 小时 |
| DNS 刷新权限处理（polkit） | 2 小时 |
| 多发行版测试（Ubuntu/Arch/Fedora） | 4 小时 |
| 打包（AppImage / Flatpak） | 4 小时 |
| **总计** | **~2 天** |

---

## 建议优先级

### 立即可做（无额外开发）

1. **CLI 工具 (oneproxy.exe)**
   - 已经跨平台，只需重新编译：
     ```bash
     GOOS=darwin GOARCH=amd64 go build -o oneproxy-darwin ./cmd/oneproxy
     GOOS=linux GOARCH=amd64 go build -o oneproxy-linux ./cmd/oneproxy
     ```
   - 无 GUI，适合服务器场景

### 短期（1 天工作量）

2. **macOS GUI**
   - Qt 支持成熟
   - 托盘图标行为标准
   - 用户群体：开发者、设计师（高价值）

### 中期（2 天工作量）

3. **Linux GUI**
   - 桌面环境碎片化需要更多测试
   - 用户群体：技术用户（可接受命令行替代）

---

## 跨平台抽象层建议

如果计划长期支持多平台，建议：

### 1. 平台特定代码隔离

```go
// internal/platform/platform.go
type Platform interface {
    HideWindow(*exec.Cmd)
    FlushDNS() error
}

// internal/platform/windows.go
type WindowsPlatform struct{}
func (p WindowsPlatform) HideWindow(cmd *exec.Cmd) {
    cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// internal/platform/darwin.go
type DarwinPlatform struct{}
func (p DarwinPlatform) HideWindow(cmd *exec.Cmd) {
    // No-op on macOS
}
```

### 2. Qt 图标资源文件

当前使用文件系统的 `.ico` 文件 → 改为 Qt 资源文件（`.qrc`），自动处理格式转换：

```xml
<!-- trayapp/icons.qrc -->
<RCC>
    <qresource prefix="/icons">
        <file>green.png</file>
        <file>yellow.png</file>
        <file>red.png</file>
    </qresource>
</RCC>
```

```cpp
QIcon(":/icons/green.png")  // 跨平台
```

### 3. CMake 平台检测

```cmake
if(WIN32)
    target_link_libraries(oneproxy-tray PRIVATE Qt6::Widgets)
elseif(APPLE)
    target_link_libraries(oneproxy-tray PRIVATE Qt6::Widgets "-framework AppKit")
elseif(UNIX)
    # Linux: check for AppIndicator
    find_package(PkgConfig)
    pkg_check_modules(APPINDICATOR appindicator3-0.1)
    if(APPINDICATOR_FOUND)
        target_link_libraries(oneproxy-tray PRIVATE ${APPINDICATOR_LIBRARIES})
    endif()
endif()
```

---

## 结论

| 平台 | 可行性 | 工作量 | 推荐度 |
|------|--------|--------|--------|
| **Windows** | ✅ 已完成 | - | - |
| **macOS** | 🟢 高 | 1 天 | ⭐⭐⭐⭐⭐ |
| **Linux** | 🟡 中 | 2 天 | ⭐⭐⭐ |

**推荐策略：**
1. 立即发布 Windows 版本（主要市场）
2. 下一版本添加 macOS 支持（开发者友好，Qt 成熟）
3. Linux 作为社区贡献目标（CLI 已可用，GUI 可选）

**最小改动跨平台方案：**
- 只发布 CLI 工具（`oneproxy` 二进制），已经跨平台
- GUI 保持 Windows 专用，标注 "GUI for Windows only, use CLI on macOS/Linux"
- 社区有需求时再添加其他平台 GUI
