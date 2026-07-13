# Release v0.2.0 - Initial Release 🎉

## What's New

**OneProxy** - A multi-port proxy aggregator that exposes multiple proxy servers as independent local SOCKS5 ports with fixed bindings.

### 🌟 Highlights

- **All Proxies Work Simultaneously** - No switching between nodes needed
- **Fixed Port Bindings** - Each port permanently bound to a specific proxy
- **Auto Health Monitoring** - Health checks every 60 seconds with latency display
- **Auto DNS Recovery** - Automatic DNS flush on failure (30-60s recovery vs 2-5min)
- **Real-time UI** - System tray with health status (✓ ✗) and latency

### 🚀 Quick Example

```bash
# Start OneProxy
./oneproxy.exe

# Use different proxies for different apps
curl -x socks5://127.0.0.1:10801 https://google.com  # Proxy 1
curl -x socks5://127.0.0.1:10802 https://google.com  # Proxy 2
curl -x socks5://127.0.0.1:10803 https://google.com  # Proxy 3
```

All running at the same time!

## 📦 Installation

### Prerequisites

1. Download `oneproxy.exe` from Assets below
2. Download sing-box:
   ```bash
   # Windows
   download-singbox.bat
   ```

### Setup

1. Create `config.json` from example:
   ```bash
   cp configs/config.example.json config.json
   ```

2. Edit with your proxy information:
   ```json
   {
     "proxies": [
       {
         "name": "Server1",
         "enabled": true,
         "local_port": 10801,
         "type": "shadowsocks",
         "server": "your-server.com",
         "port": 5299,
         "method": "aes-256-gcm",
         "password": "your-password"
       }
     ]
   }
   ```

3. Run `oneproxy.exe` and start from tray icon

## 🎯 Use Cases

- **Development Testing** - Test multiple proxy nodes simultaneously
- **Multi-App Split** - Different applications use different proxies
- **Fast Switching** - Just change port number when a node fails
- **Team Sharing** - Share proxies in LAN without exposing passwords

## 📊 Features

### Phase 1: Core
- ✅ Multi-port parallel (10801-10899)
- ✅ Fixed port-to-proxy binding
- ✅ Automatic sing-box config generation
- ✅ System tray UI

### Phase 2: Health & Recovery
- ✅ Health checker (every 60s)
- ✅ Latency measurement
- ✅ Auto DNS flush on failure
- ✅ Real-time status display
- ✅ Manual controls

## 📈 Performance

- **Memory**: ~12 MB
- **CPU**: <1% idle
- **Recovery Time**: 30-60 seconds (vs 2-5 minutes traditional)

## 📚 Documentation

See full documentation in the repository:
- [README.md](README.md) - Complete guide
- [QUICKSTART.md](QUICKSTART.md) - 5-minute start
- [RELEASE_NOTES.md](RELEASE_NOTES.md) - Detailed release notes

## 🆚 vs Traditional Clients

| OneProxy | Traditional (NekoBox/Hiddify) |
|----------|-------------------------------|
| All nodes work simultaneously | Only one node at a time |
| Change port to switch | Click GUI to switch |
| Auto DNS flush | Manual recovery |
| Multi-app split friendly | Single application focus |

## 🐛 Known Issues

None. Please report issues on GitHub.

## 🔮 Roadmap

- Notifications
- Copy address feature
- Traffic statistics
- Auto-start on boot

## 🙏 Credits

- [sing-box](https://github.com/SagerNet/sing-box) - Proxy engine
- [systray](https://github.com/getlantern/systray) - System tray

## 📄 License

MIT License

---

**Full Changelog**: This is the initial release.

**Download**: See Assets below ⬇️
