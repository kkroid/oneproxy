# OneProxy 0.8.2：确认域名迁移与 DNS 核验

## 实施边界

2026-10-26 Controller 的脱敏只读核验确认，五个 VLESS 节点的 URI fragment 经百分号解码后精确符合：

```text
^JMS-[0-9]+@(c702s[0-9]+\.portablesubmarines\.com):([0-9]+)$
```

核验证据在本次交接目录的 `redacted-subscription-sample.md`。实现只在完整匹配且备注端口等于 URI 端口时，将捕获的域名作为 `server`。URI 的端口、UUID、Reality 公钥/short-id、TLS SNI、指纹和 flow 不变。SNI 不用作连接地址。普通备注、其他域名、端口不一致、溢出端口、前后附加内容及双重编码均不触发替换；继续使用 URI host。测试账号均为虚构数字。

已有订阅更新流程仍先下载和验证，再应用和保存。旧 IP 节点通过既有名称匹配迁移；对于已登记订阅、被用户改名的 VLESS 节点，额外使用“旧 IP + 原订阅名称 + 类型 + 端口”重构旧订阅 key。该分支仅用于旧 IP 到确认域名的迁移，订阅源变更和手动节点仍遵循既有规则。这样即使本次 URI IP 同时变化，也能保留本地名称、端口、启用状态以及手动选择。

旧身份对应多个节点时拒绝本次更新，保留原配置，不猜测归属。重复条目去重、同名节点既有处理和重复更新幂等均有回归。没有稳定旧 key、同时改过本地名称和 URI IP 的历史节点无法可靠恢复身份；不新增按凭据猜测匹配。

## 日志

- DLL 启动更新前初始化应用日志，使首次启动的订阅失败也有记录。
- 启动、手动和周期更新的已提交结果记录 `subscription update succeeded: status=updated|unchanged proxies=N`；失败保留安全的 `subscription update failed`。原运行更新的增删改计数继续保留。
- 地址变化记录节点摘要、旧/新地址类型及摘要，例如 `old=ipv4:<hash> new=domain:<hash>`。摘要为 SHA-256 前 8 字节，地址摘要涵盖端口；不记录原名称、原 IP、域名、订阅 URL/token、UUID、密码或 Reality 密钥。
- CLI 启动在 stdout 输出同样的成功/变化记录；失败沿用脱敏错误并输出 stderr。订阅失败及选择恢复失败不会把上游错误原文写入应用日志。
- 核心输出滚动与解析/连接超时分类由独立日志任务在 `internal/logger`、`internal/proxy/manager*` 实现；本提交不改这些路径，也不更改 logger 的 Info/Warn/Error API。

## DNS 证据与结论

只读取主根 `bin/sing-box.exe` 的版本信息，并用它启动临时目录中的独立测试进程，未修改或停止用户在用核心。二进制构建信息：sing-box **v1.13.14**，Go **1.26.4**，源码 revision `25a600db24f7680ad9806ce5427bd0ab8afe1114`。

隔离测试 `TestSingBoxDomainDNSCacheAndFailure` 使用实际生成的 VLESS 配置，仅将 DNS 服务器地址改为本地临时 TCP DNS 服务。入站和假远端都绑定回环随机端口，域名为 `dns-fixture.test`，不读取真实配置或订阅。验证结果：

| 步骤 | 累计 DNS 查询 | 累计连接假远端 |
|---|---:|---:|
| 首次 A 记录，TTL=2s | 1 | 1 |
| TTL 内再次连接，缓存命中 | 1 | 2 |
| 过期后两次 SERVFAIL，均重查且不拨旧 IP | 3 | 2 |
| 两次 NXDOMAIN，SOA TTL/MinTTL=2s，负缓存命中 | 4 | 2 |
| 负缓存到期后恢复正常答复 | 5 | 3 |

核心日志明确出现 `lookup dns-fixture.test: SERVFAIL` 和 `lookup dns-fixture.test: NXDOMAIN`，与 `dial tcp ...: i/o timeout` 的连接失败阶段不同。DNS 传输自身也可能超时，诊断不能只见 `timeout` 就归为节点连接超时。该区别已交日志角色处理。

同版本源码 `dns/client.go`、`common/dialer/dialer.go`、`common/dialer/resolve.go` 与实验一致：域名 server 经配置的 DNS resolver 查询；默认启用缓存并按 TTL 过期；NXDOMAIN 使用 SOA 负 TTL；SERVFAIL 不缓存。测试不把本地 2s TTL 当成真实服务商的 TTL。没有证据表明要禁用缓存、改写 TTL、加入定时重启或恢复失败自动清系统 DNS。TTL 作用于后续解析和新连接，不会搬迁已经建立的 TCP 连接。

2026-10-26 **17:35:30 +08:00**，通过直接 TCP socket 向 `8.8.8.8:53` 发送公开域名 `example.com` 的 A 查询，得到匹配请求 ID 的 DNS 响应，rcode=0、2 条答案，耗时 **0.392s**。没有使用 HTTP 代理或真实订阅。机器路由/VPN等底层环境保持原样；这是当时的可达性证据，不保证未来或其他网络持续可达。结构化证据为交接目录的 `dns-direct-probe.json`。

因此本次保留 `tcp://8.8.8.8`、`detour=direct`、`ipv4_only` 及默认缓存策略。现有 legacy DNS 配置在 1.13.14 由 manager 的兼容环境变量支持；1.14 删除旧格式的迁移不在本次范围。

现场旧 IP 超时、启动更新后恢复是已知现象，不能据此断言所有历史每日故障均为同一根因。

## 版本与验证

安装器 AppVersion/PE 版本、DLL 默认版本、签名脚本默认安装包路径和当前卸载升级测试期望改为 0.8.2。build.ps1 的安装包验签路径直接使用已有 installer metadata，避免另一个硬编码。托盘仍通过 DLL `OneProxy_GetVersion` 显示版本，Windows tray/DLL 的 PE 资源仍由既有构建过程从 installer metadata 生成。未批量替换历史文档。

```powershell
go test -p 1 ./... -count=1
go test -race ./internal/config ./cmd/oneproxy-dll -run 'TestDomain|TestVLESSConfirmed|TestSubscriptionUpdateMessages' -count=1
$env:ONEPROXY_TEST_SINGBOX = 'C:/Users/kkroid/Documents/OneProxy/bin/sing-box.exe'
go test ./internal/config -run '^TestSingBoxDomainDNSCacheAndFailure$' -v -count=1
```

不设置 `ONEPROXY_TEST_SINGBOX` 时核心实验明确跳过；普通单元测试不依赖已安装核心。DNS 实验不验证真实 Reality 会话成功，假远端故意关闭连接。启动测试使用假子进程的原子快照完成信号，避免把 manager 的进程存活观察误认为测试快照已写完。

PowerShell 已修改脚本通过语法解析及版本元数据核对。主根安装包构建、最终 PE/托盘实物验证、SHA256 和日志任务集成验证由 PM 在独立审核/集成后执行；域名角色没有运行安装、修改用户配置、停止在用代理或触碰现场大日志。

完整串行包回归 `go test -p 1 ./... -count=1` 通过。此前默认包并发的全量运行出现过 Windows 测试夹具时序失败：启动快照读取已按上述完成信号修正；`TestForegroundManagerRejectsEarlyExitWithoutRestart` 的既有 500ms 提前退出窗口曾报告 `Start() error = <nil>`，单独复核及串行全量随后通过。该 manager 测试未在本提交改动，已交日志角色/审核关注。交接目录 `domain-test-evidence.txt` 保留所有失败及重跑结果，不用通过的重跑抹去先前失败。
