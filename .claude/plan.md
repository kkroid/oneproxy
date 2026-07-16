# 长期使用完善计划

## 实现列表

| # | 功能 | 改动文件 | 方式 |
|---|------|----------|------|
| 1 | 开机自启 | `trayapp/main.cpp` | HKCU Run 注册表项，菜单 toggle |
| 2 | 崩溃自动重连 | `internal/proxy/manager.go` | monitor() 检测异常退出后自动 restart，最多 3 次，每次间隔 5s |
| 4 | 端口冲突弹通知 | `trayapp/main.cpp` | autoStart() 中 pStart 失败的 err 包含 "bind"/"address" 时弹出 `showMessage` |
| 5 | 日志轮转(5天) | `internal/proxy/manager.go` | Start() 时扫描 `logs/*.log`，删除 mtime 超过 5 天的 |
| 7 | 配置错误弹通知 | `trayapp/main.cpp` | autoStart() 中 pStart 失败的 err 总是弹出 `showMessage` |
| 8 | 菜单开机自启开关 | `trayapp/main.cpp` + `i18n.h` | i18n 加 autoStart 文本，rebuildMenu() 加 toggle action |

## 实现细节

### 1. 开机自启
- 注册表路径: `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
- 键名: `OneProxy`
- 值: tray exe 完整路径
- `isAutoStart()` — 检查键是否存在
- `setAutoStart(bool)` — 写入/删除注册表键
- 菜单项显示为 toggle（选中状态 = 已启用自启）

### 2. 崩溃自动重连
- manager.go: `monitor()` 检测到非主动退出时，不设置 `m.isRunning = false`
- 改为尝试 `m.Start()` 最多 3 次，每次间隔 5 秒
- 超过 3 次才放弃

### 4/7 端口冲突 + 配置错误通知
- tray `autoStart()` 中，err 非空时统一调用 `tray->showMessage("Error", err, Critical)`
- DLL 的 Start 返回的错误文本自然包含 "bind: Only one usage of each socket address" 等有用信息

### 5. 日志轮转
- manager.go Start() 时，filepath.Glob("logs/*.log") + os.Stat 查 mtime
- >120h 则 os.Remove

### 8. 菜单开机自启开关
- i18n.h 加 `autoStart` 字段: 中文"开机自启" / 英文 "Auto-start on boot"
- rebuildMenu() 中添加 toggle action

## 验证

1. 构建 DLL + tray
2. Start → 等 5s → taskkill sing-box → 观察是否自动重启
3. 重启 Windows → 看是否自动启动
4. 生成 6 天前的 log 文件 → Start → 观察是否被删除
5. 把 sing-box 端口改成冲突 → Start → 观察通知
