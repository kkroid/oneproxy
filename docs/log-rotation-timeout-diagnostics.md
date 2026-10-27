# 超时诊断字段

应用健康日志不再复制原始 URL、节点名或错误字符串。`node_id` 是名称 SHA-256 的前 12 个十六进制字符；`local_port` 对应固定节点入口。核心日志中的 `node_id` 使用 outbound tag 计算，因此不与健康日志 ID 直接等同。

- 健康失败、恢复各按节点最多每分钟一条，首次失败也会记录。`failure_duration` 从本轮连续失败第一次探测开始计算，恢复后清零；不是远端故障的精确起始时间。
- `elapsed` 为本次健康 HTTP 请求耗时。`target_tls_handshake` 表示目标站 TLS，`request_write` 表示已完成 TLS 但尚未成功写出请求，`response_headers` 表示请求写出后等待响应头。SOCKS 隐藏的节点 DNS、TCP、代理握手均为 `unknown`；`dial_ip=unknown` 不使用本地 SOCKS 地址冒充远端地址。
- 核心摘要只提取经过 IP 解析验证的 `dial tcp IP:port:` 地址，阶段为 `tcp_connect`；它是错误中明确报告的拨号地址，不保证是配置中的节点地址。DNS 错误阶段为 `dns`。EOF、reset 无充分阶段证据，保持 `unknown`。
- 核心 `elapsed` 仅来自合法连接上下文时长字段，表示核心记录的连接经过时间，并非某个阶段的精确用时。缺失时为 `unknown`。DNS、连接超时各每分钟最多一条，EOF/reset 共用每分钟一条的限额。

摘要不打印订阅地址、凭据、原始错误或目标域名。私有核心原始日志仍保持既有行为，不可直接分享或提交 Git。日志继续复用现有 Writer，日期切分、64 MiB、3 个备份、5 天保留策略均未改变。

应用健康检查不直接控制核心自动选择。统一端口 selector 的 auto 委托 sing-box URLTest；固定节点端口不经过 auto。应用显示健康和 URLTest 的测试周期、请求与阈值不同，某次健康失败不代表核心立即更换节点。单次测速成功也不保证持续业务连接成功。
