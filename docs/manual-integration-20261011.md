# 手动组合候选（待独立审核）

授权：本 run `manual-integration-20261011-request.txt`，原 Executor 执行；不改旧结果、pending、分支或模型。主仓基线 `2c1cd5b6471c047aa6fbffe62533ee19f9daa17c`，分支 master。主仓原始草稿和 AGENTS.md 原样留存，候选在独立 worktree。

## 保全与来源

备份：`C:/Users/kkroid/.paseo/oneder-pipeline/runs/oneproxy-ubuntu2404-cli-20260907-01/manual-backup-20261011-113216`。
manifest.json SHA256：`225fc22f6acc8da2365e2f6ec52e1ee48fe16f823a650043ba15c50cf569ef81`。
包含 8 tracked dirty、AGENTS.md 与 supervisor 两文件共 11 个文件的原始字节/摘要、Git status、完整 binary diff、原 index、refs、已验证 repository.bundle。恢复方法见 manifest；优先在独立目录 clone bundle/checkout 原 HEAD，再复制 files 原字节及恢复 staged.patch。本次无暂存，主仓未改动无需恢复。

| 来源 | 候选映射 | 核实 |
| --- | --- | --- |
| f35b13574d2e2c996d2e9eb8582a3f60a4f6ba3b | 9d8cfac82ad567d34af4cc7b83bbc2f853e41390 | 单父 2c1cd5b；tree 与已 PASS 的 015a7a9219b4f2709037aa685a50962060b329e6 相等，仅集成一次 |
| 64836181430d95589278455261678472fcdb55ac | e2132edafef7a6076bdafa560de0ebf079cbacd3 | 订阅第一提交，父 2c1cd5b |
| 14193807423e4ac7ede13a93238562942a83e6d2 | 91e0dbdba90bbf43eeea748c39c51a0ed846fc34 | 父 6483618；两提交净范围为四个订阅文件；独立 PASS 覆盖完整链 |

正式 PASS：本 run `repair-20260919-integration/impl-audit/r1-a0-w0-dispatch/repair-20260919-integration-impl-audit-r1-result.json` 与 `subscription-name-conflict/impl-audit/r2-a0-w0-recovery/subscription-name-conflict-impl-audit-r2-result.json`。旧 PASS 不等于本候选 PASS。

## 每处组合决定

- DLL main.go：采用草稿 startProxy 的先停止订阅调度、等待旧健康检查、停止旧 Manager 再替换 globals；保留已审 stale-lifecycle 错误、gConfig 清空、watchManagerLifecycle 意外退出清理、手动 DNS 注入测试入口及 gMu 串行化。Stop/Restart 错误传播两边等价，不重复实现。
- DNS 冲突：保留已审显式手动恢复、系统 DNS 失败即返回、不重启、lastAttempt 节流及 lastFlush 仅成功记账。草稿“全挂自动刷新”和“系统刷新失败继续重启”被明确取代：全挂不构成本地 DNS 故障证据，失败不应产生额外代理重启。草稿失败冷却/可注入函数由已审实现覆盖。Unix 的权限提示、命令错误/输出保留，但权限不足不自动重启。
- Manager 冲突：保留已审 operations 锁、ready 观测窗口、每次自动重启均观测早退及进程错误诊断；草稿旧 deadline/直接 Wait 算法不回退。保留草稿重启前 process.close，回收前一轮 supervisor pipe。启动窗口、终态锁内关闭 channel 已由已审源码覆盖。
- CLI/manager_test 重叠无文本冲突：加入 supervisor.Main 分派，生产 CLI 与测试 helper 均闭合；repair C 的随包 CLI 保留。
- health.go：保留草稿 CheckAll 注册/串行化、Stop 等待回调、带超时 ContextDialer 和连接回收。调整草稿调度循环：不因暂时不 ready 就永久退出；CheckAll 跳过不 ready，终态由 lifecycle watcher Stop，避免自动重启后失去健康检查。
- reloadAndRestart：保留草稿先 Stop/SetConfig 再检查 Manager 状态，订阅失败回滚时即使 Manager 已停也同步旧健康检查配置；已停止代理只保存配置，仍不隐式启动。沿用订阅错误返回和旧配置落盘；运行核心若重启失败，回滚并不保证再次启动成功，此边界不伪称已恢复运行态。
- process_unix/supervisor：完整保留 owner pipe、独立子进程组、TERM→2 秒→KILL、Wait 回收及 Windows stub。Unix 原测试改为观察真实子进程 PID 被回收和 supervisor 正常退出，等待期限覆盖 2 秒升级；不再把 supervisor 当 sing-box 断言 SIGKILL。
- 新发现的订阅旧会话响应：停止 token 在调度取配置前、串行更新前和 fetch 后检查；fetch 前后 gConfig 身份不同即拒绝。保留完整身份匹配、共享端点、去重、端口/启用状态及选择器逻辑。测试模拟相同 URL 的会话替换，响应不得写入新会话。
- AGENTS.md 不纳入提交、不改字节；无关/忽略的本机配置及运行文件不读取或复制。

## 锁序与验证

订阅更新顺序 subscriptionUpdateMu → gMu；Start/Stop 持 gMu 时只关闭 subscriptionStop，不等待网络请求或获取 subscriptionUpdateMu，避免反向等待。健康检查 Stop 在释放 stateMux 后等待 wg；回调包含在 round 内，当前 DLL/CLI 均不注册自动 DNS 回调，不会从健康回调反取 gMu。Manager operations → mutex，等待进程回收前释放 mutex，supervisor close 只关闭 pipe、不等待进程。

新增健康测试覆盖 Stop 等待回调和排队检查禁入；新增订阅测试覆盖旧响应/已停止调度拒绝；新增 Unix 测试覆盖 pipe 关闭后的 child reap。Windows Go test/vet/race 及 CLI/c-shared DLL 编译证据、Linux 分包执行结果见本 run 手动 implementation JSON。

未操作真实代理；开始 PID 为 oneproxy-tray 15864、sing-box 20736（2026-10-10 21:12:30/31）。主根 Installer 须在原 Reviewer 独立复核并手动集成后执行，记录新包时间/大小/SHA256/源码提交；本候选不拿旧安装包冒充新交付。未安装、push、签名或执行 macOS 构建。
