# 0.8.2 日志滚动与核心诊断

## 范围与默认值

应用 `logger.Info/Warn/Error` API 不变；应用与核心 stdout/stderr 共用
`internal/logger.Writer`，默认活跃文件 10MiB、`.1` / `.2` / `.3` 三份备份。
按实际写入字节计数，达到边界后，在写入下一字节前关闭句柄并滚动。
单次超大写入也会拆分，因此新生成的每份文件不超过 10MiB；行和 UTF-8
字符可能跨文件，按 `.3 → .2 → .1 → 活跃文件` 拼接后还原字节顺序。
每条日志路径要求只有一个 Writer 所有者，不支持两个 OneProxy 实例共享同一路径。

核心不继承日志文件句柄：stdout/stderr 经 `os/exec` 管道进入同一个串行化写入器。
进程 Wait 排空输出后才关闭 Writer；自动重启复用 Writer，显式停止/重新启动重新
打开并读取实际大小。未改变 Job、停止、自动恢复次数或生产启动观察窗口。
删除旧的“超过五天删除目录中所有 `.log`”逻辑，避免它删除活跃日志或其他日志。

## 超大旧日志与磁盘上界

仅当新版本实际收到输出、发现活跃文件 **大于** 10MiB 时，将其原地重命名为
`<日志路径>.oversized`，然后创建新的活跃文件。不读入全文、不复制、不截断。
这个固定归档不参加滚动，不会自动删除。迁移成本与旧文件长度无关。

正常每条日志路径新增滚动预算最多 **40MiB**；两条日志合计最多 **80MiB**。
保全的旧文件占用另计。例如旧核心日志 1.43GB 得到保全后，上界为该旧文件
实际字节数加 40MiB（以及独立的应用日志预算）。升级前已经存在的其他备份和
归档属于已有占用；旧编号备份按正常保留顺序逐步淘汰，新版不会任意扫描删除它们。

如果 `.oversized` 已存在且活跃文件也超限，不覆盖任意一份，也不创建新的时间戳
归档、不继续追加。两份原有证据保持不变；受阻期间该路径不再增长。通过独立
stderr 即时提示 `log persistence blocked; manually archive the conflicting
.oversized file before retry` 及缺失字节数，后续最多每分钟一次。运维确认保全后，
将冲突的 `.oversized` 移到用户选定的归档位置，后续输出会重新尝试迁移和落盘。
用户主动移走的归档不属于本程序管理的 40MiB 预算。

本次只在临时目录模拟此流程，**没有重命名、截断或删除用户现场约 1.43GB 日志**。
真正迁移发生在用户以后启动新版且产生输出时；旧版正在运行的日志不受本次开发影响。

## 故障与恢复

关闭自身句柄后才 Rename。每个删除/重命名错误均检查；失败的轮转步骤保留，
解除 Windows 文件共享限制后从同一步恢复，避免重试不断删除剩余备份。
写入失败关闭坏句柄，下次尝试重新打开并 Stat，避免永久 nil sink。

运行期间的写入、重新打开、轮转或保全冲突失败不会停止核心输出排空。
磁盘故障期间不无限缓存、不突破磁盘上界，缺失字节计数累计在内存中；每条路径
最多每秒尝试一次磁盘恢复，其间输出继续被消费。stderr 首次立即、之后最多每分钟
报告一次；核心日志还向应用 Logger 发固定提示，不包含上游错误文本，且不递归写
故障核心 Writer。即使应用日志也受阻，两个 Writer 仍分别报告到独立 stderr。

恢复后在恢复的文件写入 `[oneproxy log recovered: N bytes lost during logging
failure]`，核心另向可用的应用日志报告恢复与缺失计数。缺失数据不能重建，不宣称
“磁盘满仍零丢失”。没有进一步输出时不主动创建后台重试线程；计数按 Writer 生命周期
维护，不跨应用崩溃持久化。显式 Close 后不会自动复活 Writer。

限制：仍同步执行操作系统文件调用；测试证明返回错误不会停止排空/死锁，不证明
永久不返回的故障驱动 I/O 可被抢占。独立 stderr 在有控制台或捕获宿主时可见；
无 stderr 的 GUI 宿主且所有磁盘日志均不可写时，没有新增系统事件日志/弹窗通道。
首次创建/打开日志失败仍按原有语义返回初始化错误；已运行核心的日志故障会重试。
0600/0700 保留现有私有文件意图，Windows ACL 仍依赖宿主目录，未新增 ACL 管理。

## 诊断和敏感信息边界

原始 `singbox.log`、备份、固定归档和内存尾部继续保留核心原文，可能包含地址、
URL 和其他敏感信息；这些是既有私有诊断边界，不应直接公开上传。
新应用诊断与 `Check` / `Start` / `Done` 返回的核心文本使用固定分类，不复制上游原文。

DNS 分类采用 sing-box 1.13.14 的实际格式：域名角色在隔离核心与本地 TCP DNS 服务中
观察到 `lookup <domain>: SERVFAIL` / `lookup <domain>: NXDOMAIN`。测试使用虚构
`proxy.invalid`。`dns: exchange failed … i/o timeout`、`lookup … i/o timeout`
归为 `dns_resolution_failed`，不会误归为 `connection_timeout`；后者识别
`connection: … dial tcp … i/o timeout` / `connectex: … timed out` 等连接超时文本。
这只是诊断分类，不证明故障根因，也不触发 DNS 清理或核心重启。
每类应用诊断每分钟最多一条，不带域名、IP、节点 tag 或任何上游参数。

启动尾部仍保留每次尝试最新 4KiB。公开错误保留退出状态与端口占用、配置解析、
权限、文件缺失、证书/密钥错误等安全分类。未知不透明文本不能靠正则证明脱敏，
因此只返回固定摘要及 `GetLogs` 查看指引，避免传播无标签密码。
`GetLogs(N)` 在 Check 或进程退出后返回此次尝试的原始内存尾部，故障落盘、跨
滚动边界也能读取；运行时沿用活跃文件读取。该 API 是**私有、未脱敏**接口，
调用方不可把其结果直接写到公开日志。超长/没有换行的输出按 4KiB 块分类，
跨块的极端超长错误可能无法分类，但不泄漏原文。

## 验证

所有日志故障、并发、超大文件、核心子进程测试仅使用 `t.TempDir()` 和测试辅助进程。
未安装、停止用户当前代理、修改用户配置或 push。

| 验收项 | 自动化证据 |
|---|---|
| 精确边界、大写入、最多三份备份 | `TestWriterExactBoundaryAndLargeWriteRetention` |
| 字节计数、多字节字符及原 Logger API | `TestLoggerUsesBytesAndDefaultPolicy` |
| 并发完整性、重新打开、Close 不复活 | `TestWriterConcurrentRecordsAndReopen` |
| 多次重启保全旧日志、固定归档冲突、限频独立提示、人工解除恢复 | `TestOversizedPreservedAcrossRestartsAndConflictRecovery` |
| 实际坏句柄写失败、重新打开失败、恢复缺失摘要 | `TestWriterWriteAndReopenFailuresRecover` |
| Windows 活跃/备份共享句柄阻止 Rename、精确阶段恢复 | `TestWindowsActiveRenameFailureKeepsBoundAndRecovers` / `TestWindowsBackupRenameFailureResumesExactStep` |
| 核心两个输出流超过 10MiB、重复启动滚动与尾部 | `TestManagerConcurrentCoreStreamsRotateAcrossRestart` |
| 应用与核心同时受阻、核心继续运行、人工解除恢复、再次重启 | `TestManagerBothLogsBlockedRecoverWithoutStoppingCore` |
| 错误写入不终止排空、4KiB 尾部仍可读取 | `TestProcessOutputDrainsFailureAndPreservesPrivateTail` |
| 实际 DNS 格式、DNS 超时和连接超时区分、不透明敏感参数 | `TestCoreDiagnosticFormatsAndPrivacy` |
| 分片/并发输出、脱敏和诊断限频 | `TestProcessOutputSplitAndConcurrentDiagnosticsAreSanitized` |
| 未知启动/配置检查错误公开安全、私有尾部可读 | `TestManagerUnknownStartupErrorRetainsPrivateTail` / `TestManagerCheckUnknownErrorRetainsPrivateTail` |

最终命令、结果和限制记录在任务 `logs-handoff.json`。Windows 原生测试及 race
覆盖本改动；未声称 Linux/macOS 运行验证或真实磁盘满、故障驱动压力验证。
早期退出分支测试原来将 Windows 新测试 EXE 的调度/初始化限制在 500ms，20 次
并发回归中复现两次 Start 返回 nil（一次耗时 3.45s），并有失败后未清理导致句柄占用。
该分支测试改为 10s 测试观察上限且添加 Stop 清理，收到退出即返回；默认 1s 生产窗口
和其独立观察测试保持不变，没有增加生产 sleep 或放宽启动保证。
