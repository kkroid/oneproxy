# OneProxy v0.2.0 - Initial Release 🎉

## 🚀 What is OneProxy?

OneProxy is a **multi-port proxy aggregator** that exposes multiple proxy servers (like JustMySocks) as independent local SOCKS5 ports with fixed bindings. Unlike traditional proxy clients that switch between nodes, OneProxy allows **all proxy nodes to work simultaneously** on different ports.

### Key Concept

```
JustMySocks 5 nodes (with complex passwords/configs)
                ↓
        OneProxy manages them
                ↓
    Exposes as 5 simple SOCKS5 ports
                ↓
127.0.0.1:10801 → Server1 (fixed binding)
127.0.0.1:10802 → Server2 (fixed binding)
127.0.0.1:10803 → Server3 (fixed binding)
127.0.0.1:10804 → Server4 (fixed binding)
127.0.0.1:10805 → Server5 (fixed binding)
```

**All ports work at the same time - no switching needed!**

---

## ✨ Features

### Phase 1: Core Features

- **Multi-Port Parallel** - All proxy nodes exposed simultaneously on different ports
- **Fixed Binding** - Each port permanently bound to a specific node (no auto-switching)
- **Password Hidden** - Users only need port numbers, upstream passwords are hidden
- **Automatic Config** - Generates sing-box configuration automatically
- **System Tray UI** - Easy control with start/stop/restart buttons
- **Port Conflict Detection** - Validates unique port assignment

### Phase 2: Health Monitoring & Auto Recovery

- **Health Checker** - Independent health check for each node (every 60 seconds)
- **Latency Measurement** - Real-time latency display in milliseconds
- **Auto DNS Flush** - Automatic DNS cache flush on failure (3 consecutive failures)
- **Fast Recovery** - 30-60 seconds recovery time vs 2-5 minutes traditional
- **Real-time UI** - Health status icons (✓ ✗) and latency updated every 5 seconds
- **Manual Controls** - Manual health check and DNS flush buttons

---

## 🎯 Use Cases

### 1. Development Testing
Test multiple proxy nodes simultaneously:
```bash
for port in 10801 10802 10803; do
    curl -x socks5://127.0.0.1:$port -w "Time: %{time_total}s\n" https://google.com &
done
```

### 2. Multi-Application Split
Different apps use different nodes:
```
Chrome Profile 1 → 127.0.0.1:10801 (YouTube)
Chrome Profile 2 → 127.0.0.1:10802 (Twitter)
VSCode         → 127.0.0.1:10803 (Extensions)
Terminal       → 127.0.0.1:10804 (Git/npm)
```

### 3. Fast Switching
Node down? Just change the port number - no GUI, no config edit, no restart.

### 4. Team Sharing
Share proxies within LAN without exposing upstream passwords:
```json
"inbound": {
  "listen": "0.0.0.0",  // Allow LAN access
  "proxy_type": "socks5"
}
```
Team members use: `192.168.1.100:10801`

---

## 📦 What's Included

### Executables
- `oneproxy.exe` - Main program (Windows)
- sing-box binary (download separately)

### Configuration
- `configs/config.example.json` - Example configuration with 5 nodes
- Supports Shadowsocks, VMess, Trojan, and more

### Tools
- `download-singbox.bat` - Auto-download sing-box (Windows)
- `download-singbox.sh` - Auto-download sing-box (Linux/macOS)
- `Makefile` - Build scripts

### Documentation
- `README.md` - Complete project documentation
- `QUICKSTART.md` - 5-minute quick start guide
- `DEVELOPMENT.md` - Developer documentation
- `TECHNICAL_INVESTIGATION.md` - sing-box technical research
- Multiple implementation reports

---

## 🚀 Quick Start

### 1. Download sing-box
```bash
# Windows
download-singbox.bat

# Linux/macOS
./download-singbox.sh
```

### 2. Configure Proxies
```bash
cp configs/config.example.json config.json
# Edit config.json with your proxy information
```

Example configuration:
```json
{
  "proxies": [
    {
      "name": "JMS-Server1",
      "enabled": true,
      "local_port": 10801,
      "type": "shadowsocks",
      "server": "c331s1.portablesubmari",
      "port": 5299,
      "method": "aes-256-gcm",
      "password": "your-password"
    }
  ],
  "health_check": {
    "enabled": true,
    "interval_seconds": 60
  },
  "dns": {
    "flush_on_failure": true
  }
}
```

### 3. Run
```bash
./oneproxy.exe
```

### 4. Start Proxy
Right-click tray icon → "启动所有代理"

### 5. Use
```bash
# Browser: Configure SwitchyOmega with 127.0.0.1:10801
# Terminal:
curl -x socks5://127.0.0.1:10801 https://google.com
curl -x socks5://127.0.0.1:10802 https://google.com
```

---

## 📊 System Requirements

- **OS**: Windows 10/11 (primary), Linux/macOS (ready)
- **sing-box**: v1.13.14 or later
- **Memory**: ~12 MB
- **CPU**: <1% idle, 2-5% during health checks
- **Network**: ~5 KB/min for health checks

---

## 🆚 Comparison

| Feature | NekoBox/Hiddify | OneProxy |
|---------|-----------------|----------|
| **Design** | Single exit, auto-switch | **Multi-port, fixed binding** |
| **Usage** | Only one node at a time | **All nodes simultaneously** |
| **Switching** | GUI clicks | **Change port number** |
| **Recovery** | Manual | **Auto DNS flush** |
| **Use Case** | Daily browsing | **Dev testing, multi-app split** |

---

## 🔧 Configuration Options

### Health Check
```json
"health_check": {
  "enabled": true,
  "interval_seconds": 60,      // Check every 60 seconds
  "timeout_seconds": 5,         // Request timeout
  "test_url": "https://www.google.com/generate_204"
}
```

### DNS Configuration
```json
"dns": {
  "flush_on_failure": true,              // Auto flush on failure
  "flush_interval_seconds": 300,         // Periodic flush interval
  "servers": [
    "https://1.1.1.1/dns-query",
    "https://8.8.8.8/dns-query"
  ]
}
```

### Inbound Configuration
```json
"inbound": {
  "listen": "127.0.0.1",    // Local only (secure)
  // "listen": "0.0.0.0",   // LAN access (team sharing)
  "proxy_type": "socks5"    // socks5, http, or mixed
}
```

---

## 📱 Tray Menu

```
OneProxy - 运行中 🟢
├── 启动所有代理
├── 停止所有代理
├── 重启所有代理
├── ───────────────
├── 代理列表 ▶
│   ├── ✓ JMS-Server1 [:10801] (45ms)
│   ├── ✓ JMS-Server2 [:10802] (67ms)
│   ├── ✗ JMS-Server3 [:10803] (失败)
│   ├── ✓ JMS-Server4 [:10804] (52ms)
│   └── ✓ JMS-Server5 [:10805] (89ms)
├── ───────────────
├── 健康检查: 已启用 (每60秒)
├── 立即检查所有节点
├── DNS刷新: 已启用
├── 立即刷新DNS
├── ───────────────
├── 打开配置文件
├── 查看日志
└── 退出
```

---

## 🐛 Known Issues

None at this time. Please report issues on GitHub.

---

## 🔮 Roadmap

### Phase 3 (Future)
- [ ] Windows Toast notifications
- [ ] Copy address to clipboard
- [ ] Built-in log viewer
- [ ] Config hot reload
- [ ] Traffic statistics
- [ ] Auto-start on boot
- [ ] Subscription link import

---

## 📝 Documentation

- **README.md** - Complete project documentation
- **QUICKSTART.md** - 5-minute quick start guide
- **DEVELOPMENT.md** - Developer documentation
- **TECHNICAL_INVESTIGATION.md** - Technical research on sing-box
- **IMPLEMENTATION_REPORT.md** - Phase 1 implementation
- **PHASE2_REPORT.md** - Phase 2 implementation
- **PROJECT_SUMMARY.md** - Project summary

---

## 🙏 Credits

- [sing-box](https://github.com/SagerNet/sing-box) - Powerful proxy engine
- [systray](https://github.com/getlantern/systray) - System tray library

---

## 📄 License

MIT License - See [LICENSE](LICENSE) file for details

---

## 🤝 Contributing

Contributions are welcome! Please read [DEVELOPMENT.md](DEVELOPMENT.md) for development guidelines.

---

## 📧 Support

- **Issues**: https://github.com/kkroid/oneproxy/issues
- **Documentation**: See docs folder

---

## ⚠️ Disclaimer

Please comply with local laws and regulations. Use this tool only for legitimate purposes.

---

**OneProxy - Multi-Port Proxy Aggregator, Making Proxy Management Easier!** 🚀

---

**Release Date**: 2026-07-13  
**Version**: v0.2.0  
**Status**: Production Ready ✅
