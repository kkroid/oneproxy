# OneProxy

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)
[![Qt Version](https://img.shields.io/badge/Qt-6.8-41CD52?logo=qt)](https://www.qt.io/)

Multi-port proxy aggregator for Windows — converts multiple upstream proxy nodes (Shadowsocks/VMess) into independent local SOCKS5 ports, managed via a native C++ Qt6 system-tray GUI.

## Architecture

```
Upstream Proxies (Shadowsocks/VMess)
        │
        ▼
  oneproxy.dll  ←─── Go core: config parser, sing-box manager, health check, DNS flush
        │
        ├─→ oneproxy-tray.exe  (C++ Qt6 system tray GUI) ← Primary interface
        │
        └─→ oneproxy.exe       (Go CLI, optional)
```

**Key Features:**
- 🎯 **Multi-port mapping** — Each upstream node → dedicated local SOCKS5 port (e.g., :10801, :10802...)
- 🩺 **Health monitoring** — Automatic latency checks, visual status indicators
- 🔄 **DNS management** — Auto-flush system DNS + sing-box restart on failures
- 🪟 **Native GUI** — Qt6 system tray, no console windows, lightweight (~2 MB)
- ⚙️ **Powered by sing-box** — Battle-tested proxy core with protocol diversity

---

## Screenshots

### System Tray Menu
<kbd>![Tray Menu](docs/screenshots/tray-menu.png)</kbd>

Shows all proxies with:
- ✓ Health status (green/yellow/red indicators)
- Port numbers and latency in ms
- Start/Stop/Restart controls
- Manual health check and DNS flush triggers

### Health Check in Action
<kbd>![Health Status](docs/screenshots/health-status.png)</kbd>

Automatic background checks every 60s (configurable). Failed nodes trigger DNS flush and retry.

---

## Quick Start

### Prerequisites

| Component | Version | Purpose |
|-----------|---------|---------|
| **Windows** | 10/11 | Target OS |
| **Go** | 1.21+ | Build DLL |
| **MSVC** | 2022 | Build C++ tray |
| **Qt** | 6.8+ | GUI framework |
| **sing-box** | Latest | Proxy engine |

### Installation

#### 1. Download sing-box

```powershell
# Download from https://github.com/SagerNet/sing-box/releases
# Extract sing-box.exe to OneProxy/bin/
mkdir bin
# Place sing-box.exe in bin/
```

#### 2. Configure Proxies

```powershell
# Copy example config
cp configs\config.example.json config.json

# Edit config.json with your proxy details
notepad config.json
```

Example configuration:

```json
{
  "proxies": [
    {
      "name": "US-Node-1",
      "enabled": true,
      "local_port": 10801,
      "type": "shadowsocks",
      "server": "us1.example.com",
      "port": 8388,
      "method": "aes-256-gcm",
      "password": "your-actual-password"
    }
  ]
}
```

See [docs/configuration.md](docs/configuration.md) for full reference.

#### 3. Build

```powershell
# One-command build (requires MSVC 2022 + Qt6 in PATH)
.\build.ps1
```

If build fails due to missing paths, see [docs/installation.md](docs/installation.md) for manual setup.

#### 4. Run

```powershell
.\trayapp\build\oneproxy-tray.exe
```

The tray icon appears in the system tray (bottom-right). Right-click to open menu.

---

## Usage

### Verify Proxies

```powershell
# Test SOCKS5 port with curl
curl -x socks5://127.0.0.1:10801 https://ip.sb
# Should return your proxy's exit IP
```

### Configure Browser

**Firefox:**
1. Settings → Network Settings → Manual proxy configuration
2. SOCKS Host: `127.0.0.1`, Port: `10801`
3. SOCKS v5: ✓

**Chrome/Edge:**
```powershell
# Launch with proxy (replace 10801 with your port)
chrome.exe --proxy-server="socks5://127.0.0.1:10801"
```

**System-wide (Windows):**
1. Settings → Network & Internet → Proxy
2. Manual setup → SOCKS proxy: `127.0.0.1:10801`

### HTTP/HTTPS Proxy Support

OneProxy defaults to **SOCKS5** proxies. To use **HTTP/HTTPS** proxies instead:

#### Method 1: Change Global Type (Recommended)

Edit `config.json` and set `inbound.proxy_type` to `http`:

```json
{
  "inbound": {
    "listen": "127.0.0.1",
    "proxy_type": "http"
  }
}
```

After restarting, all ports become HTTP CONNECT proxies:

```powershell
# Test HTTP proxy
curl -x http://127.0.0.1:10801 https://ip.sb

# Browser configuration
# Firefox: HTTP Proxy 127.0.0.1:10801
# Chrome: --proxy-server="http://127.0.0.1:10801"
```

#### Method 2: Mixed Mode

Set different types per proxy in `config.json`:

```json
{
  "proxies": [
    {
      "name": "SOCKS5-Node",
      "local_port": 10801,
      "inbound_type": "socks5"  // This port is SOCKS5
    },
    {
      "name": "HTTP-Node",
      "local_port": 10802,
      "inbound_type": "http"    // This port is HTTP
    }
  ]
}
```

**Trade-offs:**
- HTTP proxies **do not support UDP** (e.g., DNS queries), SOCKS5 does
- Some applications (e.g., Telegram) only support SOCKS5
- Performance: SOCKS5 is slightly faster (simpler protocol)

### Routing Modes

OneProxy supports three routing modes, selectable from the tray menu:

| Mode | Behavior |
|------|----------|
| **Global** | All traffic goes through the proxy (default) |
| **Rule** | China IPs/domains → direct, everything else → proxy |
| **Direct** | All traffic goes directly, proxy bypassed |

Rule mode uses sing-box's built-in `rule_set` router with community-maintained databases:

| Database | Size | Purpose |
|----------|------|---------|
| `geoip.db` | 4 MB | IP address → country mapping |
| `geosite.db` | 3.5 MB | Domain → category mapping (cn, ads, etc.) |

These databases are maintained by the [SagerNet community](https://github.com/SagerNet/sing-geoip/releases) and can be updated independently by replacing the files in `bin/`. They are automatically copied to `~/.oneproxy/` on startup.

**Rule mode decision flow:**
```
Request to www.google.com  → geosite check → NOT "cn" → routed through proxy
Request to www.baidu.com   → geosite check → IS "cn"   → direct connection
Request to 119.29.29.29     → geoip check  → IS "cn"   → direct connection
Request to 8.8.8.8          → geoip check  → NOT "cn"  → routed through proxy
```

This is **server-side routing** — it affects all traffic passing through the proxy, not just browser traffic. Unlike PAC scripts which work at the browser level, sing-box routing works for any application using the proxy.

### Tray Menu Actions

| Action | Effect |
|--------|--------|
| **启动所有代理** | Start all enabled proxies |
| **停止所有代理** | Stop sing-box, close all ports |
| **重启所有代理** | Restart + trigger health check |
| **立即检查所有节点** | Manual health check (bypasses 60s interval) |
| **立即刷新 DNS** | Flush system DNS + restart sing-box |
| **退出** | Stop proxies and quit |

**Icon Colors:**
- 🟢 Green — All proxies healthy
- 🟡 Yellow — Some proxies timeout
- 🔴 Red — All down or stopped

---

## API Reference

### CLI (oneproxy.exe)

```powershell
# Start all proxies
.\oneproxy.exe start

# Stop all proxies
.\oneproxy.exe stop

# Show status (JSON)
.\oneproxy.exe status

# Health check
.\oneproxy.exe check

# Flush DNS
.\oneproxy.exe flush
```

### DLL Exports (oneproxy.dll)

```c
char* OneProxy_Start(char* configPath);     // Returns error or NULL
char* OneProxy_Stop();
char* OneProxy_Restart();
char* OneProxy_Status();                    // JSON string
char* OneProxy_HealthCheck();
char* OneProxy_FlushDNS();
void  OneProxy_FreeString(char* ptr);       // Free returned strings
```

See `cmd/oneproxy-dll/main.go` for implementation.

---

## Troubleshooting

### sing-box.exe not found
```
Error: failed to start sing-box: exec: "bin/sing-box.exe": file does not exist
```
**Solution:** Download sing-box from [releases](https://github.com/SagerNet/sing-box/releases) and place in `OneProxy/bin/`.

### Port already in use
```
Error: listen tcp 127.0.0.1:10801: bind: Only one usage of each socket address
```
**Solution:** Another process is using the port. Change `local_port` in `config.json` or kill the conflicting process:
```powershell
netstat -ano | findstr :10801
taskkill /PID <PID> /F
```

### Health check always fails
```
All proxies show red, latency = timeout
```
**Causes:**
1. Upstream server down — check with provider
2. Incorrect password/UUID in config.json
3. Firewall blocking outbound connections — add sing-box.exe to Windows Firewall exceptions
4. DNS poisoning — run "立即刷新 DNS" from tray menu

Check logs:
```powershell
type logs\singbox.log
```

### Tray icon not showing
- Qt platform plugin missing — ensure `platforms\qwindows.dll` exists in build dir
- Run from PowerShell to see error output:
```powershell
cd trayapp\build
.\oneproxy-tray.exe
# Check console output
```

More issues? See [docs/troubleshooting.md](docs/troubleshooting.md)

---

## Project Structure

```
OneProxy/
├── cmd/
│   ├── oneproxy/          # Go CLI executable
│   └── oneproxy-dll/      # Go DLL (C shared library)
├── internal/
│   ├── config/            # Config parser + sing-box config generator
│   │   ├── config.go      # JSON unmarshal, validation
│   │   └── singbox.go     # Generate sing-box JSON from config
│   └── proxy/             # Core proxy management
│       ├── manager.go     # Process lifecycle, Start/Stop/Restart
│       ├── health.go      # Health checker with timeout
│       └── dns.go         # DNS flusher (ipconfig /flushdns + restart)
├── trayapp/               # C++ Qt6 system tray
│   ├── CMakeLists.txt     # MSVC build config
│   ├── main.cpp           # QSystemTrayIcon + DLL FFI
│   └── *.ico              # Green/yellow/red status icons
├── configs/
│   └── config.example.json
├── bin/                   # sing-box binary (user provides)
├── logs/                  # sing-box stdout/stderr
├── build.ps1              # One-click build script
└── config.json            # User config (gitignored)
```

---

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for:
- Setting up development environment
- Build system details (Go build modes, Qt/CMake, MSVC toolchain)
- Code structure and conventions
- Running tests
- Submitting PRs

---

## Performance

- **Memory footprint:** ~15 MB (oneproxy-tray.exe + DLL + Qt runtime)
- **CPU usage:** <1% idle, <5% during health checks
- **Latency overhead:** ~5-10ms per proxy hop (local SOCKS5 relay)
- **Concurrent connections:** Limited by sing-box (typically 1000+ per port)

---

## Deployment Checklist

To deploy oneproxy-tray.exe to another Windows machine:

**Required files:**
```
trayapp/build/
├── oneproxy-tray.exe
├── oneproxy.dll
├── config.json              # Your proxy config
├── green.ico, yellow.ico, red.ico
├── Qt6Core.dll, Qt6Gui.dll, Qt6Widgets.dll
├── platforms/
│   └── qwindows.dll         # Qt platform plugin
└── bin/
    └── sing-box.exe         # Proxy engine
```

**Optional:**
- `oneproxy.exe` — CLI tool
- `logs/` — Auto-created for sing-box output

---

## License

[MIT License](LICENSE) — Copyright (c) 2026 OneProxy Contributors

---

## Acknowledgments

- **[sing-box](https://github.com/SagerNet/sing-box)** — Universal proxy platform
- **[Qt](https://www.qt.io/)** — Cross-platform GUI framework
- **[JustMySocks](https://justmysocks.net/)** — Example upstream provider (not affiliated)

---

## FAQ

**Q: Why not use v2rayN or Clash?**  
A: OneProxy exposes each node as a separate port, enabling per-application proxy routing without profile switching.

**Q: Does this work on macOS/Linux?**  
A: The Go DLL core is cross-platform, but the Qt tray app currently targets Windows only. PRs welcome for other platforms.

**Q: Can I use HTTP proxies instead of SOCKS5?**  
A: Change `proxy_type` to `"http"` in `config.json` → `inbound` section. sing-box will create HTTP CONNECT proxies on the same ports.

**Q: How do I add a new proxy node?**  
A: Edit `config.json`, add a new entry to `proxies` array with a unique `local_port`, restart via tray menu.

**Q: Is this faster than using a VPN?**  
A: SOCKS5 proxies have lower overhead than VPNs (no TUN/TAP layer), but speed depends on your upstream server quality.
