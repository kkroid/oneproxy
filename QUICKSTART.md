# OneProxy 快速入门指南

本指南帮助你在 5 分钟内完成 OneProxy 的配置和运行。

## OneProxy 是什么？

OneProxy 是一个**代理端口聚合器**，将多个代理服务器（如 JustMySocks）同时暴露为独立的本地 SOCKS5 端口。

**简单理解**：
- 你有 5 个 JustMySocks 节点，每个都有复杂的密码和配置
- OneProxy 启动后，它们变成 5 个简单的本地端口：10801、10802、10803...
- 不同应用可以同时使用不同端口，互不干扰

## 步骤 1: 下载 sing-box

### 方法 A: 使用自动下载脚本（推荐）

**Windows:**
```bash
# 双击运行或在命令行执行
download-singbox.bat
```

**Linux/macOS:**
```bash
./download-singbox.sh
```

### 方法 B: 手动下载

1. 访问 [sing-box Releases](https://github.com/SagerNet/sing-box/releases)
2. 下载对应你系统的版本（Windows 选择 `*-windows-amd64.zip`）
3. 解压后将 `sing-box.exe` 放入项目的 `bin/` 目录

## 步骤 2: 配置代理信息

### 2.1 创建配置文件

```bash
# 从示例配置复制
cp configs/config.example.json config.json
```

### 2.2 编辑配置文件

打开 `config.json`，**关键是配置 local_port 字段**：

**Shadowsocks 示例:**
```json
{
  "proxies": [
    {
      "name": "JMS-Server1",
      "enabled": true,
      "local_port": 10801,              // 本地暴露端口（必填）
      "type": "shadowsocks",
      "server": "c331s1.portablesubmari",  // 推荐使用域名
      "port": 5299,
      "method": "aes-256-gcm",
      "password": "你的密码"
    },
    {
      "name": "JMS-Server2",
      "enabled": true,
      "local_port": 10802,              // 必须唯一
      "type": "shadowsocks",
      "server": "c331s2.portablesubmari",
      "port": 5299,
      "method": "aes-256-gcm",
      "password": "你的密码"
    }
  ],
  "inbound": {
    "listen": "127.0.0.1",    // 仅本机访问（安全）
    "proxy_type": "socks5"     // 统一输出 SOCKS5
  }
}
```

**VMess 示例:**
```json
{
  "name": "JMS-Server3",
  "enabled": true,
  "local_port": 10803,
  "type": "vmess",
  "server": "c331s3.portablesubmari",
  "port": 5299,
  "uuid": "你的UUID",
  "alter_id": 0,
  "security": "auto"
}
```

### 2.3 配置说明

**关键字段**：
- `local_port` - **每个代理必须配置独立的端口号**（如 10801, 10802...）
- `server` - **推荐使用域名**而不是 IP（JustMySocks 会自动更新域名解析）
- `enabled` - 是否启用此代理

**inbound 配置**：
- `listen: "127.0.0.1"` - 仅本机可用（推荐）
- `listen: "0.0.0.0"` - 允许内网其他设备访问

## 步骤 3: 运行 OneProxy

### 3.1 构建（首次运行）

```bash
go build -o oneproxy.exe ./cmd/oneproxy
```

### 3.2 启动程序

**Windows:**
```bash
# 双击 oneproxy.exe 或命令行运行
./oneproxy.exe
```

### 3.3 使用系统托盘

程序启动后，在系统托盘（右下角）显示图标：

1. 右键点击托盘图标
2. 点击"启动所有代理"
3. 图标变为绿色 🟢
4. 菜单中可以看到每个节点的端口号

## 步骤 4: 使用代理

### 方法 A: 浏览器插件（推荐）

使用 [SwitchyOmega](https://chrome.google.com/webstore/detail/proxy-switchyomega/padekgcemlokbadohgkifijomclgjgif)：

**创建多个情景模式**：

```
Profile-Server1:
  协议: SOCKS5
  服务器: 127.0.0.1
  端口: 10801

Profile-Server2:
  协议: SOCKS5
  服务器: 127.0.0.1
  端口: 10802

Profile-Server3:
  协议: SOCKS5
  服务器: 127.0.0.1
  端口: 10803
```

然后根据需要切换使用不同的 Profile。

### 方法 B: 命令行工具

```bash
# 使用节点1（端口 10801）
curl -x socks5://127.0.0.1:10801 https://google.com

# 使用节点2（端口 10802）
curl -x socks5://127.0.0.1:10802 https://google.com

# Git 使用节点1
git config --global http.proxy socks5://127.0.0.1:10801

# npm 使用节点2
npm config set proxy socks5://127.0.0.1:10802
```

### 方法 C: 应用程序

大多数应用支持代理设置：

```
代理类型: SOCKS5
代理地址: 127.0.0.1
代理端口: 10801 (或其他配置的端口)
```

### 快速复制地址

右键托盘图标 → 节点列表 → 右键某个节点 → 复制地址

## 步骤 5: 多代理并行使用

**OneProxy 的核心优势：所有代理同时可用**

```bash
# 同时测试所有节点
for port in 10801 10802 10803 10804 10805; do
    echo "Testing port $port"
    curl -x socks5://127.0.0.1:$port -w "\nTime: %{time_total}s\n" https://google.com &
done
wait
```

**实际应用场景**：

```
Chrome窗口1 → SwitchyOmega Profile1 → 10801 (看YouTube)
Chrome窗口2 → SwitchyOmega Profile2 → 10802 (看Twitter)
VSCode     → Settings → 10803 (下载插件)
Terminal   → export all_proxy=socks5://127.0.0.1:10804
```

**所有应用同时工作，使用不同节点，互不干扰！**

## 测试验证

### 检查端口是否正常

```bash
# 测试节点1
curl -x socks5://127.0.0.1:10801 https://ip.sb

# 测试节点2
curl -x socks5://127.0.0.1:10802 https://ip.sb

# 应该显示不同节点的IP地址
```

### 查看延迟

右键托盘图标 → 节点列表，可以看到：
```
✓ JMS-Server1 [:10801] (45ms)
✓ JMS-Server2 [:10802] (67ms)
✗ JMS-Server3 [:10803] (超时)
```

## 常见问题

### Q1: 提示"sing-box binary not found"

确保 `bin/sing-box.exe` 文件存在。运行 `download-singbox.bat` 下载。

### Q2: 提示"local_port conflicts"

每个代理的 `local_port` 必须唯一。检查 config.json 中是否有重复端口。

### Q3: 某个节点连不上

1. 检查托盘菜单中该节点的状态（✓ 或 ✗）
2. 查看日志：右键托盘 → 查看日志
3. 尝试换另一个端口（如果其他节点正常）

### Q4: 如何知道用的是哪个节点？

每个端口固定绑定一个节点：
- 10801 → JMS-Server1
- 10802 → JMS-Server2
- 10803 → JMS-Server3

右键托盘图标可以看到完整列表。

### Q5: 能不能自动切换到最快节点？

**不能，也不会**。OneProxy 的设计理念是：
- 所有节点同时暴露
- 用户手动选择端口
- 不自动切换，精确控制

如果需要自动切换，请使用 NekoBox 或 Hiddify。

### Q6: 可以分享给内网其他设备吗？

可以。修改 config.json：
```json
"inbound": {
  "listen": "0.0.0.0",    // 改成 0.0.0.0
  "proxy_type": "socks5"
}
```

内网设备使用 `你的局域网IP:10801` 即可。

## 进阶使用

### 应用级分流

为不同应用分配不同节点：

```bash
# .bashrc 或 .zshrc
alias proxy1='export all_proxy=socks5://127.0.0.1:10801'
alias proxy2='export all_proxy=socks5://127.0.0.1:10802'
alias noproxy='unset all_proxy'

# 使用
proxy1  # 切换到节点1
curl https://google.com

proxy2  # 切换到节点2
curl https://google.com
```

### 快速故障切换

```bash
# 某个节点挂了？
# 不需要重启 OneProxy，不需要修改配置
# 只需要把应用的端口号改一下

# 从 10801 改成 10802
# 几秒钟搞定
```

### 同时测速

```bash
# 找出最快的节点
for port in 10801 10802 10803 10804 10805; do
    echo -n "Port $port: "
    curl -x socks5://127.0.0.1:$port -o /dev/null -s -w "%{time_total}s\n" https://google.com
done
```

## 下一步

恭喜！你已经成功配置了 OneProxy。

**推荐操作**:

1. **配置更多节点** - 把 JustMySocks 的所有节点都配置进来
2. **设置浏览器** - 用 SwitchyOmega 创建多个 Profile
3. **测试延迟** - 找出最快和最稳定的节点
4. **等待 Phase 2** - 健康检查和自动 DNS 刷新功能

**获取帮助**:

- 完整文档: `README.md`
- 开发文档: `DEVELOPMENT.md`
- 项目状态: `PROJECT_STATUS.md`

---

**OneProxy 的独特价值**：不是通用代理客户端，而是为多代理并行、精确控制设计的专用工具。
