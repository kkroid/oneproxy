# Windows 安装升级

安装程序在用户确认目标目录之后、复制文件之前停止该目录的 OneProxy。Ready 页面说明代理会短暂中断；配置文件仍使用 `onlyifdoesntexist`，订阅及用户数据不参与覆盖。

## 退出与覆盖保护

`trayapp/platform/upgrade_windows.cpp` 编译成静态 CRT 的 x86 辅助 DLL，由 Inno 的 32 位脚本宿主从私有临时目录加载。它不加载安装目录中的 Qt 或 Go DLL。

1. 对安装目录和进程映像使用 Windows 最终路径规范化。仅接管该目录的 `oneproxy-tray.exe` / `oneproxy.exe`，保留进程句柄避免 PID 复用。子进程还须符合父 PID、创建时间和该目录的 `bin/sing-box.exe` 路径。递归核查后代；无法验证或出现未知子进程时拒绝继续。Windows 自己的 conhost 只等待自然退出，不强杀。
2. 新托盘注册 `OneProxy.UpgradeQuit.v1` 并将退出排入 Qt 事件循环，沿用已有 `aboutToQuit` 中的启动取消、线程回收和 Stop。已识别为支持该协议的进程超时不强杀，安装返回失败。
3. 旧托盘的 `OneProxyTrayWindow` 不处理退出，不能把 WM_CLOSE 当成 Qt 退出。向该窗口所属 GUI 线程投递 WM_QUIT，等待真实进程句柄变为退出状态。旧版或尚无窗口的目标进程若仍不退出，才对已验证并持有的精确句柄兜底终止；再次检查并等待已确认的子进程。没有按通用进程名执行 taskkill。
4. 全程持有目标目录的 `.oneproxy-upgrading` 文件，使用 delete-on-close 自动清理；新托盘（含 self-check）和 CLI 在加载核心/启动代理之前拒绝进入升级中的目录。
5. 停止之后，对旧应用 EXE、DLL、SRS、图标、占位配置取得写句柄并拒绝读共享。存在映像映射、外部文件占用或写权限拒绝时不能准备成功。这些句柄保持到复制完成，禁止旧版在扫描与复制之间重新加载，同时允许 Inno 替换文件。新文件由升级标记保护。失败释放句柄；用户可重试或取消，不能选择跳过准备失败而继续覆盖。
6. 根目录的 `uninsNNN.exe/dat/msg` 属于 Inno 卸载事务，单独检查读、写和删除权限并取得独占句柄。准备失败释放全部句柄；准备成功后在 `ssInstall` 仅释放卸载信息句柄，交由 Inno 读取/追加 DAT、替换 EXE 和更新注册项。应用文件句柄和升级标记仍保持到 `ssPostInstall`。不能把释放点放在 `ssPostInstall`：Inno 的 `Saving uninstall information` 早于该事件。没有删除卸载器、关闭卸载或跳过失败。

卸载器不能当作普通应用进程终止。当前 Inno 真实卸载器在确认对话框阶段就占有 DAT，回归中由 DAT 的共享冲突（Windows error 32）拒绝升级，同时原卸载器保持存活。外部 EXE/DAT 文件锁同样在准备阶段拒绝；只读属性和显式写/删除 ACL 拒绝报告 error 5。错误包含文件路径，用户可关闭卸载窗口/解除占用后重试或取消。`PrepareToInstall` 仅返回错误时，Inno 没有 Retry 按钮；现在显式显示 Retry/Cancel 对话框，Retry 重新执行全部准备检查，取消返回准备失败，静默模式默认取消（exit 7）。检查针对准备阶段已存在的冲突；交还 Inno 后新增的外部锁、ACL 变化和恶意目录修改不提供跨进程事务保证，后续失败由 Inno 原生重试/取消和回滚处理，不能承诺任意并发修改下的原子升级。

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
- 完整 Inno 安装验证使用单独编译的隔离变体：随机独立 AppId、真实卸载 EXE/DAT 和 HKCU 注册项，关闭快捷方式、安装后启动和历史安装目录读取，降低权限到当前用户，将配置目的地转向测试目录。安装升级代码、辅助 DLL 和新应用载荷使用实际实现。生产 setup 只构建，不在真实用户环境执行。未验证 UAC 提权、真实 Program Files ACL 或生产 AppId 的卸载登记。
- 真实旧 EXE/DLL 升级、文件锁阻止复制/释放锁后重试、最终载荷哈希、配置及订阅哨兵哈希由完整 setup 验证；路径识别、无 Job 旧进程兜底、孤儿清理、独立同名实例不受影响、同目录无归属 sing-box 不被误杀、新版停止超时、写权限拒绝、新实例争用由原生辅助程序回归验证。

## 本轮证据（2026-10-12）

证据根目录：`C:/Users/kkroid/.codex/handoffs/oneproxy-windows-upgrade-20261012`。

- `pre-startup-order-build` 旧托盘 SHA256：`CDD81774AEBD04B387AD4A029C936AAF597CE62E4C3EF98165E70FF42E7F9B16`；旧核心：`04246836BF327EE67312504E8BB0CCE1D7542875AAB7269BBAF2B20584EBD9DA`。
- b761cb6 托盘：`C26A416FB61C802784FF54C56EC5504713FD532E6A461D8A571A8381DE1715DA`；核心：`75D70723675D78B8F8CB2E5EDD204AEB9FBE2CC109C3CFCC1C57ABABF5AB3CF4`。只读核对确认与现用 `C:/Temp/OneProxy-install-r2` 的两个文件相同。
- 两套旧 Qt 的 WM_QUIT 正常退出、Job 子进程退出，以及新版协议均有真实进程句柄证据；强制兜底单独使用无 Job 夹具验证。
- `impl-build-final2.log`：执行工作树生产 setup 编译；`go-tests.log`：CLI、DLL 和 proxy 包测试；`qt-tests/verification.log`：现有 Qt/启动取消回归 4/4。
- `verification-final2/verification.log`：最终原生与完整隔离 setup 回归；`setup-legacy/--setup-locked.log`、`setup-baseline/--setup-locked.log` 记录占用导致准备失败；对应 `--setup.log` 记录释放锁后完成替换。早期 `verification-1` 至 `verification-3` 及 `verification-final` 保留隔离方法调查与 32/64 位路径修正时的失败，不能充当通过证据。

执行者只提交实现及验证材料。独立 Reviewer PASS 后负责实际合并；随后 PM 仍须在主项目根执行 `pwsh -NoProfile -File ./build.ps1 -Installer`，报告该次主仓 setup 路径和 SHA256。本工作树生成物不替代最终主仓交付。

## 卸载信息冲突回归（2026-10-12）

旧 `verification-final2` 禁用了卸载生命周期，不能作为此问题的通过证据。`test-windows-upgrade.ps1` 现改为调用 `test-windows-uninstall-upgrade.ps1`，分别覆盖两套旧版先安装、运行后升级，且不再使用 `Uninstallable=no`。单独执行方式：

```powershell
pwsh -NoProfile -File scripts/test-windows-uninstall-upgrade.ps1 `
  -LegacyBuild '<旧版只读制品目录>' `
  -OutputDirectory '<新的独立证据目录>'
```

证据根目录：`C:/Users/kkroid/.codex/handoffs/oneproxy-uninstall-upgrade-20261012`。

- `investigation/install2.log`：最小真实安装器二次安装在保存卸载信息时复现 code 5；`share-read.log` 表明仅增加读共享仍失败。原生 `--handle-test` 在没有任何其他进程占用的文件上，以原访问/共享标志直接复现 MoveFileEx code 5，关闭该句柄后替换成功，并验证修复的卸载句柄释放不会解除 DLL/标记保护。
- `verification-2/verification.log`：真实旧版安装、基线错误复现、外部 EXE/DAT 锁、只读属性、DAT ACL 拒绝、运行中的真实卸载器均已验证。各准备失败前后比较卸载 EXE/DAT、注册值、应用载荷及配置/订阅哈希；解锁后旧 Qt 优雅退出、升级与卸载成功。运行中卸载器的占用来自 DAT，不能只检查 EXE。
- `verification-final3-console.log` 与 `verification-final3`：完整原生退出/精确进程身份/保护回归，以及两套旧制品的真实卸载生命周期与私有 desktop 下的向导 Retry/Cancel 回归。额外在 `Starting the installation process` 后才锁 EXE，验证保存失败 exit 5，取消后原 EXE/DAT/注册值及本次同版本载荷哈希不变，解除锁后再次安装成功。测试仅关闭自身 Job 内创建的进程，清理通过测试目录中自己的卸载器进行，不修改生产 AppId 注册项。
- `verification-1`、`verification-full`、`verification-final`、`verification-final2` 和 `ui-probe` 留存注册名缩短、UI 定位、缺失 Retry 按钮及自动化重复点击误判的调查失败，不作为整套通过证据；`ui-final` 验证新增对话框取消 exit 7、同一向导解锁 Retry exit 0。
- `build-final.log`：最终代码的工作树 `build.ps1 -Installer` 构建记录。主仓构建、SHA256 和交付仍由独立审核合并后的 PM 完成。
