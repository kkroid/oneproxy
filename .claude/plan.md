# OneProxy 开源上线 - 完成总结

## ✅ 第一批已完成（文档 + 国际化 + 跨平台分析）

| # | 任务 | 状态 |
|---|------|------|
| 1 | README.md（英文） | ✅ 完整重写：架构图、安装、API、FAQ、故障排查、部署清单 |
| 2 | LICENSE | ✅ MIT License |
| 3 | config.json 凭据检查 | ✅ git 历史安全，无泄露 |
| 9 | .gitignore 补全 | ✅ 添加 Qt Creator 临时文件 |
| 10 | config.example.json | ✅ 清理为通用占位符 |
| 13 | Release 产物说明 | ✅ 已整合到 README "Deployment Checklist" |
| - | **README.zh.md（中文）** | ✅ 完整中文文档 |
| - | **HTTP/HTTPS 代理说明** | ✅ 已添加到两份 README，含混合模式示例 |
| - | **菜单国际化** | ✅ 实现 `trayapp/i18n.h`（QLocale 自动检测中英文） |
| - | **跨平台可行性报告** | ✅ `docs/CROSS_PLATFORM.md`（macOS 1天，Linux 2天） |

---

## 📝 待执行（第二批 - 工程化）

### #4 build.ps1 路径自动检测 ⚠️

**当前问题：**
```powershell
$qtDir = "C:\Qt\6.8.3\msvc2022_64"  # 硬编码
$vcvars = "C:\Program Files\...\2022\Community\..."  # 硬编码
```

**改进方案：**
1. Qt 检测：环境变量 `$env:QTDIR` → qmake 路径解析 → 提示手动设置
2. MSVC 检测：vswhere.exe → vcvars64.bat

### #6 windeployqt 参数评估 🟡

当前：`--no-system-d3d-compiler --no-opengl-sw`

需测试：
- 去掉参数后包体积增加
- 无独显机器渲染测试

### #11 用户文档 📚

创建 `docs/` 目录：
- [ ] `installation.md` / `installation.zh.md` — 环境准备、sing-box 下载
- [ ] `configuration.md` / `configuration.zh.md` — config.json 完整字段参考
- [ ] `usage.md` / `usage.zh.md` — 浏览器代理设置、验证连接
- [ ] `troubleshooting.md` / `troubleshooting.zh.md` — 故障排查

### #12 CONTRIBUTING.md 🤝

- [ ] 开发环境要求（Go 1.21+, Qt 6.8, MSVC 2022）
- [ ] 构建步骤详解
- [ ] 代码结构说明
- [ ] PR 规范、commit message 格式

### #5 build 目录治理 🧹

- [x] .gitignore 已排除 trayapp/build/
- [ ] 确认根目录无误入文件

---

## 🟢 改进项（可选 - 第三批）

| # | 项 | 优先级 |
|---|-----|--------|
| 14 | 菜单国际化 | ✅ **已完成** |
| 15 | build.ps1 路径自动检测 | 高（见第二批 #4） |
| 16 | CLI --version flag | 中 |
| 17 | CLI --config flag | 中 |
| 18 | 单元测试（config、singbox 生成） | 中 |
| 19 | GitHub Actions CI | 低 |
| 20 | .editorconfig | 低 |

---

## 📸 缺失截图

README 中引用但未提供：
- `docs/screenshots/tray-menu.png` — 托盘菜单（中英文各一张）
- `docs/screenshots/health-status.png` — 健康检查状态

**制作方法：**
1. 运行 `oneproxy-tray.exe`
2. 右键托盘图标
3. 截图菜单（含延迟数据）
4. 保存到 `docs/screenshots/`

---

## 🚀 发布前检查清单

### 必须项 ✅

- [x] LICENSE 文件
- [x] README.md（英文）
- [x] README.zh.md（中文）
- [x] config.example.json（无真实凭据）
- [x] .gitignore（排除敏感文件）
- [ ] 截图（2 张）
- [x] 跨平台说明（CROSS_PLATFORM.md）

### 推荐项 🟡

- [ ] docs/ 用户文档（4 个 md 文件 × 2 语言）
- [ ] CONTRIBUTING.md
- [ ] build.ps1 路径检测改进
- [ ] GitHub Release 说明（发布时撰写）

### 可选项 🟢

- [ ] 单元测试
- [ ] CI/CD
- [ ] macOS/Linux 版本

---

## 建议发布流程

### v0.3.0 初始开源版本

1. **补充截图**（10 分钟）
2. **创建 docs/ 文档**（2 小时）— 安装、配置、使用、故障排查
3. **CONTRIBUTING.md**（1 小时）
4. **build.ps1 路径检测**（1 小时）— 可选，但能降低新用户门槛
5. **最终测试**（30 分钟）— 完整构建 + 代理验证
6. **Git tag v0.3.0** + **GitHub Release**

### Release Notes 草稿

```markdown
# OneProxy v0.3.0 — Initial Release

Multi-port proxy aggregator for Windows with native Qt6 system tray GUI.

## Features
- 🎯 Multi-port SOCKS5/HTTP proxy (each node → dedicated port)
- 🩺 Auto health check with latency display
- 🔄 DNS auto-flush on failures
- 🪟 Native Qt6 GUI, no console windows
- 🌍 Chinese/English UI (auto-detect)

## Downloads
- `oneproxy-tray-windows-amd64.zip` — GUI version (recommended)
- `oneproxy-windows-amd64.exe` — CLI version (optional)

## Requirements
- Windows 10/11
- [sing-box](https://github.com/SagerNet/sing-box/releases) (place in `bin/`)

## Quick Start
See [README.md](README.md) for installation guide.
```

---

## 当前可以立即开源 ✅

核心功能完整，文档齐全。如果你：
- **接受英文文档为主**（中文已补充）
- **愿意后续补充 docs/ 详细文档**
- **可以手动截图 2 张**

那么**现在就可以 push 到 GitHub**，标记 `v0.3.0-beta` 或 `v0.3.0`。

---

## 下一步

等待你的决定：
1. **立即开源** — 我帮你生成 Git 命令 + Release 说明
2. **先补文档** — 我继续创建 docs/ 下的 8 个 md 文件
3. **其他优先级** — 告诉我你想先做什么
