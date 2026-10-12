# Windows 安装升级

安装程序在用户确认目标目录之后、复制文件之前停止该目录的 OneProxy。Ready 页面说明代理会短暂中断；配置文件仍使用 `onlyifdoesntexist`，订阅及用户数据不参与覆盖。

## 退出与覆盖保护

`trayapp/platform/upgrade_windows.cpp` 编译成静态 CRT 的 x86 辅助 DLL，由 Inno 的 32 位脚本宿主从私有临时目录加载。它不加载安装目录中的 Qt 或 Go DLL。

1. 对安装目录和进程映像使用 Windows 最终路径规范化。仅接管该目录的 `oneproxy-tray.exe` / `oneproxy.exe`，保留进程句柄避免 PID 复用。子进程还须符合父 PID、创建时间和该目录的 `bin/sing-box.exe` 路径。递归核查后代；无法验证或出现未知子进程时拒绝继续。Windows 自己的 conhost 只等待自然退出，不强杀。
2. 新托盘注册 `OneProxy.UpgradeQuit.v1` 并将退出排入 Qt 事件循环，沿用已有 `aboutToQuit` 中的启动取消、线程回收和 Stop。已识别为支持该协议的进程超时不强杀，安装返回失败。
3. 旧托盘的 `OneProxyTrayWindow` 不处理退出，不能把 WM_CLOSE 当成 Qt 退出。向该窗口所属 GUI 线程投递 WM_QUIT，等待真实进程句柄变为退出状态。旧版或尚无窗口的目标进程若仍不退出，才对已验证并持有的精确句柄兜底终止；再次检查并等待已确认的子进程。没有按通用进程名执行 taskkill。
4. 全程持有目标目录的 `.oneproxy-upgrading` 文件，使用 delete-on-close 自动清理；新托盘（含 self-check）和 CLI 在加载核心/启动代理之前拒绝进入升级中的目录。
5. 停止之后，对旧 EXE、DLL、SRS、图标、占位配置取得写句柄并拒绝读共享。存在映像映射、外部文件占用或写权限拒绝时不能准备成功。这些句柄保持到复制完成，禁止旧版在扫描与复制之间重新加载，同时允许 Inno 替换文件。新文件由升级标记保护。失败释放句柄；用户可重试或取消，不能选择跳过准备失败而继续覆盖。

禁用 Inno 的跨应用 Restart Manager 自动关闭/重启，避免它扩展停止范围；没有安排运行中的 DLL 在重启后热替换。复制完成后、可选的启动动作之前释放保护。系统掉电、恶意进程修改目标目录和任意外部程序在新文件复制后强行加载 DLL 不属于事务性回滚保证。

## 复现验证

先在执行工作树运行 `pwsh -NoProfile -File ./build.ps1 -Installer`，然后：

```powershell
pwsh -NoProfile -File scripts/test-windows-upgrade.ps1 `
  -LegacyBuild '<旧制品只读源>\trayapp\build' `
  -BaselineBuild '<b761cb6 制品只读源>\trayapp\build' `
  -OutputDirectory '<新的独立证据目录>'
```

脚本拒绝复用已有输出目录，记录源二进制 SHA256、前后进程身份和安装日志。测试使用私有 Windows desktop；清理 Job 仅包含本次创建的进程。不会停止默认桌面代理，也不会向它发退出消息。

测试隔离边界必须保留，不能称为未经修改的生产安装包实机升级：

- Qt 6.8 的 `QDir::homePath()` 优先使用进程令牌的 `GetUserProfileDirectoryW`，仅更改 USERPROFILE 无效。因此仅在暂停的测试托盘进程中加载 `upgrade_profile_shim.cpp`，替换 QtCore 的这一项 IAT 导入，指向独立测试 profile；旧 EXE、Go DLL 和磁盘上的 Qt DLL 不做修改。退出机制及 Job 实现仍来自真实旧二进制。
- 旧核心硬编码 Clash API `127.0.0.1:9090`，与现用代理冲突。测试中的 sing-box 路径放置一个不联网的长期存活子进程夹具，由真实 Go DLL 启动、分配 Job、停止。它验证真实父进程的 Job 生命周期，但不是实际 sing-box 网络运行验证；不能把短暂启动后因 9090 冲突退出的真实 sing-box 算作成功。
- 完整 Inno 安装验证使用单独编译的隔离变体：关闭卸载注册、快捷方式和历史安装目录读取，降低权限到当前用户，将配置目的地转向测试目录。安装 `[Code]`、升级辅助 DLL 和新应用载荷使用实际实现。生产 setup 只构建，不在真实用户环境执行。未验证 UAC 提权、真实 Program Files ACL、交互式按钮操作和生产卸载登记。
- 真实旧 EXE/DLL 升级、文件锁阻止复制/释放锁后重试、最终载荷哈希、配置及订阅哨兵哈希由完整 setup 验证；路径识别、无 Job 旧进程兜底、孤儿清理、独立同名实例不受影响、同目录无归属 sing-box 不被误杀、新版停止超时、写权限拒绝、新实例争用由原生辅助程序回归验证。

## 本轮证据（2026-10-12）

证据根目录：`C:/Users/kkroid/.codex/handoffs/oneproxy-windows-upgrade-20261012`。

- `pre-startup-order-build` 旧托盘 SHA256：`CDD81774AEBD04B387AD4A029C936AAF597CE62E4C3EF98165E70FF42E7F9B16`；旧核心：`04246836BF327EE67312504E8BB0CCE1D7542875AAB7269BBAF2B20584EBD9DA`。
- b761cb6 托盘：`C26A416FB61C802784FF54C56EC5504713FD532E6A461D8A571A8381DE1715DA`；核心：`75D70723675D78B8F8CB2E5EDD204AEB9FBE2CC109C3CFCC1C57ABABF5AB3CF4`。只读核对确认与现用 `C:/Temp/OneProxy-install-r2` 的两个文件相同。
- 两套旧 Qt 的 WM_QUIT 正常退出、Job 子进程退出，以及新版协议均有真实进程句柄证据；强制兜底单独使用无 Job 夹具验证。
- `impl-build-final2.log`：执行工作树生产 setup 编译；`go-tests.log`：CLI、DLL 和 proxy 包测试；`qt-tests/verification.log`：现有 Qt/启动取消回归 4/4。
- `verification-final2/verification.log`：最终原生与完整隔离 setup 回归；`setup-legacy/--setup-locked.log`、`setup-baseline/--setup-locked.log` 记录占用导致准备失败；对应 `--setup.log` 记录释放锁后完成替换。早期 `verification-1` 至 `verification-3` 及 `verification-final` 保留隔离方法调查与 32/64 位路径修正时的失败，不能充当通过证据。

执行者只提交实现及验证材料。独立 Reviewer PASS 后负责实际合并；随后 PM 仍须在主项目根执行 `pwsh -NoProfile -File ./build.ps1 -Installer`，报告该次主仓 setup 路径和 SHA256。本工作树生成物不替代最终主仓交付。
