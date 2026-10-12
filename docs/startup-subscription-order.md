# 启动前订阅更新

## 行为与入口

- 托盘自动启动、手动启动、显式重启，以及 DLL 的 `OneProxy_Start` / `OneProxy_Restart`：有订阅时先下载、解析、复用现有 `MergeSubscription` 合并与完整配置校验、原子保存已解析到的配置文件，然后生成运行配置和启动 sing-box。无订阅直接启动；首次安装仍恢复随包占位配置。
- CLI `run` 使用相同的下载/合并/校验函数，在生成和进程检查前保存 `--config` 指定的配置。CLI 原先没有周期订阅更新，本次仍保留其前台模式；托盘/DLL 的六小时周期更新不变，计时从连接成功后开始，移除了连接成功后的即时重复刷新。
- 显式重启先停止当前连接，再走完整启动流程。正在运行的会话内部定期更新、手动更新及 sing-box 崩溃恢复仍沿用原有机制，不被当作新的应用启动。没有增加启动失败后连接旧节点的回退入口。
- 下载、解析、合并、校验或保存失败：原配置不变，不生成本次运行配置、不启动连接。错误固定说明“Subscription update failed; not connected”，提示检查网络/订阅，再点击 Start（CLI 重新执行命令）。状态接口在尚未连接时也发布进度/失败，托盘不会显示原始 URL、解析错误或凭据。
- 托盘启动在线程中执行，菜单可在下载期间停止；退出会回收启动线程。DLL 下载期间释放状态锁；Stop 取消 HTTP context 并撤销启动身份。下载完成后只有同一启动身份可以保存/连接。并发 Start、启动期间导入或手动/定期更新被拒绝，已有更新队列通过配置与会话身份拒绝跨会话回写。手工节点导入也在排队前捕获身份。

## 网络行为与可用性边界

HTTP 客户端沿用 Go 默认 Transport，支持 `HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY` 环境变量及系统 DNS；不会读取 Windows Internet Options 作为 Go HTTP 代理，也没有显式指定 OneProxy 的 SOCKS/HTTP 端口。总请求超时仍是 15 秒（含重定向与读响应体）；新增 context 使启动中 Stop/CLI 信号能取消请求。订阅体上限为 1 MiB，超过上限整次失败，不能截断后当作完整订阅保存。

程序不会为了下载订阅先启动原节点。如果订阅只能经由本应用尚未启动的代理访问，或者环境代理指向已停止的本应用端口，启动会超时/失败并保持未连接；需要用户修复网络或订阅后重试。现有外部网络、VPN 或环境代理若可用，仍可用于请求。本次没有要求离线运行、禁止外部代理，也没有承诺所有订阅在所有网络下可访问。未访问真实用户订阅或外网来推断连通性。

沿用现有节点合并、同名冲突、两次缺失删除和 Windows 行为。SS 解析仍保留已有 JMS fragment 作为主机名的语义，本任务没有更改协议解析规则。

## 实际验证（Windows，2026-10-12）

`go test -race ./cmd/oneproxy ./cmd/oneproxy-dll ./internal/config -count=1` 通过；Go 原有同名冲突、失败回滚及平台测试也包含在这些包内。

| 回归 | 证据 |
| --- | --- |
| 保存后才启动、每次启动/显式重启更新一次 | `TestActualStartupSavesBeforeLaunchingAndRefreshesEveryStart` 通过真实 DLL 启动函数调用真实 Manager，子进程是复制到临时 bin 的测试可执行文件。下载阻塞时无生成文件/子进程快照；进程启动瞬间记录的持久配置和生成配置均含新节点；再次 Start/Restart 请求次数精确递增；Restart 更新失败没有遗留连接。 |
| 五类失败保全 | `TestActualStartupFailurePreservesFileAndNeverLaunches` 对 HTTP 503、带凭据的解析错误、合并冲突、无效端口和原子替换失败分别验证原始字节、内存节点不变且没有生成配置/Manager。保存失败用非空目录阻挡替换，原文件移至旁边逐字节核对。 |
| 停止、并发及旧请求 | `TestActualStartupStopConcurrentUpdatesAndReplacement` 在本地 HTTP 阻塞时执行并发 Start、手动/定期更新和导入，均拒绝；Stop 后两秒内返回取消，替代配置和状态不受影响。`TestPeriodicSessionStopRejectsLateFailure` 验证运行期更新取消/替代会话。原有排队身份与迟到响应测试继续通过。 |
| 真实有限超时 | `TestSubscriptionHTTPTimeoutAndCancellation` 的本地服务一直等待请求取消，实际等待约 15 秒并确认传输关闭；另一分支主动取消 context。另测超大响应拒绝。 |
| 首次安装、无订阅 | `TestFreshInstallWithoutSubscriptionStartsDirectly` 删除临时配置并使用临时占位文件，验证创建全新数据目录、完整恢复占位配置、直接启动测试子进程。 |
| CLI 实际入口 | `TestCLIActualEntryUpdatesBeforeGenerationAndPreservesOnFailure` 调用 `run`，更新成功后才到达生成与进程检查；失败不生成、不改文件、错误脱敏。测试环境没有旁置 sing-box，所以成功分支故意止于进程检查，不声称建立实际代理连接。 |
| 周期不重复刷新 | `TestSubscriptionPeriodicUpdaterDoesNotRefreshImmediately` 验证连接后更新器不立即请求、重复创建不替换会话，并保留六小时常量；未进行六小时实时时长等待。 |
| 托盘 | MSVC 2022 / Qt 6.8.3 Release 编译完整 tray 与测试通过；`ctest --test-dir %TEMP%/oneproxy-startup-order-qt -R oneproxy-subscription-test --output-on-failure` 通过，覆盖未连接更新进度、失败/重试文本、脱敏和重复通知抑制。没有对真实托盘实例做点击自动化。 |

CMake 使用 `-G "NMake Makefiles" -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTING=ON -DCMAKE_PREFIX_PATH=C:/Qt/6.8.3/msvc2022_64`，构建目录在系统临时目录，未把验证制品写入仓库。工具探测提示找不到独立 `vswhere.exe` 和可选 Vulkan headers，但实际 MSVC 编译、链接及所选测试均成功。

本轮没有安装、杀停现有代理、修改真实用户订阅、push 或发布。真实进程顺序夹具使用 Windows 直接子进程生命周期，非 Windows 上该夹具跳过；其他平台尚未实际验证。Go race 证据限于上述 Windows 测试路径，不代表跨进程共享配置的并发写入有文件锁（原有 CLI/DLL 跨进程互斥边界未扩展）。

## 后续交付责任

Executor 提交实现与以上测试，交独立 Implementation Reviewer 审核并实际合并。审核/合并成功后，由 PM 在 `C:/Users/kkroid/Documents/OneProxy` 根执行 `pwsh -NoProfile -File ./build.ps1 -Installer`，记录新 setup 路径、SHA256 与验证边界；本轮测试构建不是安装包交付，不复用历史制品。

## 第 2 轮：修复 Stop 与排队启动的交接

独立审核发现第一轮托盘的“Stop → join → 再 Stop”只能事后停止晚到的启动。现已移除第二次 Stop，改为跨 DLL 边界的代次协议：UI 线程先调用 `OneProxy_StartupEpoch` 捕获代次，再派发 `OneProxy_StartAtEpoch`；`stopProxy` 在 `gMu` 内递增代次，同时取消已注册的下载。晚到调用在同一把 `gMu` 内核对代次并进入启动流程，因此 Stop 若先取得锁，旧调用在读取配置、下载、保存、连接前就被拒绝；Start 若先注册下载，则 Stop 取消其 context 并撤销会话身份。没有添加调用前的无同步检查。

旧同步 `OneProxy_Start` 导出保留；新托盘要求同包的新 DLL 提供两个代次导出，缺失时加载失败，不能静默回退到存在竞态的入口。此协议只处理同一 DLL 实例内的异步启动/停止交接，不扩大到跨进程互斥。

本轮只执行新增/受修改路径验证，保留第 1 轮其余已通过证据，没有重跑全量 Go race、vet 或订阅状态测试：

- `go test -race ./cmd/oneproxy-dll -run '^TestStartupEpochStopBeforeDLLAndDuringDownload$' -count=1 -v`：通过。真实 DLL 启动逻辑 + 本地 HTTP 服务证明旧代次在 Stop 后零下载，新代次可开始下载，旧调用不能影响替代会话，下载中 Stop 两秒内取消，原配置字节不变、无生成配置/连接/迟到状态。
- 新增 `oneproxy-startup-stop-test`：CMake 在构建目录读取当前 `trayapp/main.cpp`，仅开放 private 和重命名应用 main 后加入测试入口；直接执行实际 `doStart` / `doStop` / `stopStartupWorker`，不复制这些方法。可控 DLL 边界强制覆盖进入 DLL 前与下载中两种交错，断言一次 Stop、保存/连接计数均为零、旧完成回调不改变停止状态。`ctest --test-dir %TEMP%/oneproxy-startup-stop-r2 -R oneproxy-startup-stop-test -V`：1/1 通过（offscreen，10 秒超时）。该测试使用模拟 DLL 边界；Go 回归另行验证真实 DLL 状态锁和取消语义。
- MSVC 2022 / Qt 6.8.3 Release 编译新测试及完整托盘成功；`go build -buildmode=c-shared` 在临时目录构建 DLL 成功。新托盘 `--self-check` 加载该 DLL 成功；ctypes 实际调用两个新导出及 Stop，验证 C ABI、代次失效与晚到调用返回固定取消错误，未读取任何真实配置。
- `git diff --check` 通过。所有制品仍在系统临时目录，没有安装、触碰真实订阅或现有代理。新增托盘回归是实际方法的隔离调用，不是对用户现有托盘的点击自动化。
