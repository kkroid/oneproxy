# Unified Mode 实现计划

## Context

当前 OneProxy 只有 "multi-port" 模式：每个上游代理一个独立 SOCKS5 端口，固定绑定、不切换。这是小众场景。

90% 代理用户的场景是"统一出口，自动选最快节点，可手动切换"——即 sing-box 的 `urltest` + `selector` 组合。开源项目应覆盖这个主场景。

## 改动文件

### 1. `internal/config/config.go`
- `Config` 结构体加 `Mode string`（"multi-port" 默认 / "unified"）
- `InboundConfig` 加 `Port int`（unified 模式下的统一端口，如 1080）
- `Validate()`: unified 模式跳过 local_port 唯一性检查
- `Validate()`: unified 模式要求 `Inbound.Port > 0`

### 2. `internal/config/singbox.go`
- 新增 `URLTestOutbound` 结构体
- `generateInbounds()`: unified 模式只生成 1 个 inbound（`listen_port` = `cfg.Inbound.Port`）
- `generateOutbounds()`: unified 模式在所有 proxy outbound 之上叠加：
  ```json
  { "type": "urltest", "tag": "auto", "outbounds": ["out-A","out-B",...] },
  { "type": "selector", "tag": "proxy", "outbounds": ["auto","out-A","out-B",...], "default": "auto" }
  ```
- `generateRoute()`: unified 模式 `final = "proxy"`，无 inbound 绑定规则

### 3. `configs/config.example.json`
- 加 unified 模式示例注释
- 保留 multi-port 作为默认

### 4. `trayapp/main.cpp`
- `tick()`: unified 模式下菜单显示格式不同——显示当前选中节点、延迟、可点击切换
- 有 Selector 时，menu 列出所有节点作为可点击项，当前节点标记 ✓
- 无 Selector（仅 urltest），显示 auto-choice 的节点

### 5. `cmd/oneproxy-dll/main.go`
- `Status()` 返回的 JSON 增加 `mode` 字段和 `active_proxy` 字段（当前选中/活跃节点）

### 6. `README.md` + `README.zh.md`
- "Modes" 章节说明两种模式及使用场景

## sing-box 配置示例（unified 模式）

```json
{
  "inbounds": [{
    "type": "socks", "tag": "in-unified",
    "listen": "127.0.0.1", "listen_port": 1080
  }],
  "outbounds": [
    {"type": "shadowsocks", "tag": "out-Proxy1", ...},
    {"type": "vmess", "tag": "out-Proxy2", ...},
    {"type": "vmess", "tag": "out-Proxy3", ...},
    {
      "type": "urltest",
      "tag": "auto",
      "outbounds": ["out-Proxy1", "out-Proxy2", "out-Proxy3"],
      "url": "https://www.google.com/generate_204",
      "interval": "5m"
    },
    {
      "type": "selector",
      "tag": "proxy",
      "outbounds": ["auto", "out-Proxy1", "out-Proxy2", "out-Proxy3"],
      "default": "auto"
    },
    {"type": "direct", "tag": "direct"}
  ],
  "route": {
    "rules": [{"protocol": "dns", "outbound": "direct"}],
    "final": "proxy"
  }
}
```

## 测试

1. 配置 `mode: "unified"` → 编译 → 启动
2. `curl -x socks5://127.0.0.1:1080 https://ip.sb` 应返回延迟最低节点的 IP
3. 托盘菜单显示当前活跃节点
4. 健康检查只对 1 个端口做（:1080），但分别测每个上游的延迟
