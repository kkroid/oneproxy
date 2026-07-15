# Mixed Mode v0.4.0 — 完整设计

## 菜单展示

### Mixed 模式（unified_port 已配置）

```
──────────────────────────────────────
  Unified :1080 ▶ JMS-Server3 (42ms)     ← i18n: "Unified :%1 ▶ %2 (%3ms)"
──────────────────────────────────────
  ▶ JMS-Server3    :10803  42ms          ← 点击：切换到该节点
    JMS-Server1    :10801  89ms          ← 点击：切换到该节点
    JMS-Server2    :10802  156ms
    JMS-Server4    :10804  timeout       ← 不可用(disabled)
    JMS-Server5    :10805  67ms
    JMS-Server6    :10806  1206ms
──────────────────────────────────────
🟢 Running                              ← i18n: s.running
──────────────────────────────────────
Stop All Proxies
Restart All Proxies
──────────────────────────────────────
Check All Nodes
Flush DNS
──────────────────────────────────────
Quit
```

### 关键行为

| 元素 | 行为 |
|------|------|
| **Unified 行** | 只读，显示当前活跃节点 + 延迟 |
| **▶ 节点** | 当前活跃节点，再次点击无操作 |
| **正常节点** | 可点击，点击后 unified 端口切换到该节点 |
| **超时节点** | disabled，不可点击 |

## 切换实现

sing-box selector 通过 Clash API 控制。需要新增：

### 1. sing-box 配置开启 Clash API

```json
{
  "experimental": {
    "clash_api": {
      "external_controller": "127.0.0.1:9090",
      "external_ui": ""
    }
  }
}
```

### 2. DLL 新增 API

```c
char* OneProxy_SelectProxy(char* proxyName);  // PUT /proxies/proxy → {"name":"out-XXX"}
```

Go 实现：`http.NewRequest("PUT", "http://127.0.0.1:9090/proxies/proxy", body)`

### 3. tray 点击回调

```
点击某节点 → DLL OneProxy_SelectProxy(name) → sing-box Clash API → 流量立即切到新节点
```

## 国际化

### i18n.h 新增字段

```cpp
struct Strings {
    // ... existing ...
    QString unifiedLabel;     // "Unified :%1" / "统一出口 :%1"
    QString unifiedActive;    // "▶ %1 (%2ms)" / already handled by formatting
};
```

中英文自动适配：
- 中文系统：`统一出口 :1080 ▶ Server3 (42ms)`
- 英文系统：`Unified :1080 ▶ Server3 (42ms)`

## 切换位置

用户在**菜单里直接点节点名**就切换——不需要子菜单、不需要设置页。

```
右键托盘 → 看到 7 行（1 unified + 6 节点）
                     ↓
              点 "JMS-Server1"
                     ↓
        unified 端口立即切到 Server1
                     ↓
        ▶ 标记移到 Server1 那行
```

## 实现文件

| 文件 | 改动 |
|------|------|
| `config.go` | 删 `Mode`，加 `ClashPort` 字段 |
| `singbox.go` | 新增 `ClashAPIConfig` 结构体，unified 模式加 clash_api |
| `oneproxy-dll/main.go` | 新增 `OneProxy_SelectProxy` 导出 |
| `trayapp/main.cpp` | 菜单节点可点击（unified 模式），调用 SelectProxy |
| `trayapp/i18n.h` | 新增 unifiedLabel |
| `configs/config.example.json` | 添加 mixed 模式示例 |
