# OneProxy

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![Qt Version](https://img.shields.io/badge/Qt-6.8-41CD52?logo=qt)](https://www.qt.io/)

Multi-port proxy aggregator for Windows — converts Shadowsocks, VMess, and VLESS Reality upstream nodes into independent local mixed proxy ports (SOCKS5 + HTTP CONNECT), managed through a native C++ Qt6 system-tray GUI.

## Architecture

```
Upstream Proxies (Shadowsocks / VMess / VLESS Reality)
        │
        ▼
  oneproxy.dll  ←─── Go core: config parser, sing-box manager, health check, DNS flush
        │
        ├─→ oneproxy-tray.exe  (C++ Qt6 system tray GUI) ← Primary interface
        │
        └─→ oneproxy.exe       (Go CLI, optional)
```

**Key Features:**
- 🎯 **Multi-port mapping** — Each upstream node → dedicated local SOCKS5 + HTTP port (e.g., :10801, :10802...)
- 🔐 **VLESS Reality** — Supports TCP + Reality + XTLS Vision subscriptions
- 🩺 **Health monitoring** — Automatic latency checks, visual status indicators
- 🔄 **DNS management** — Auto-flush system DNS + sing-box restart on failures
- 🪟 **Native GUI** — Qt6 system tray, no console windows, lightweight (~2 MB)
- ⚙️ **Powered by sing-box** — Battle-tested proxy core with protocol diversity

---

## Quick Start

### Prerequisites

| Component | Version | Purpose |
|-----------|---------|---------|
| **Windows** | 10/11 | Target OS |
| **Go** | 1.26+ | Build DLL |
| **MSVC** | 2022 | Build C++ tray |
| **Qt** | 6.8+ | GUI framework |
| **sing-box** | 1.13.14 | Proxy engine and rule-set compiler |

### Installation

#### 1. Download sing-box

```powershell
# Download sing-box 1.13.14 from https://github.com/SagerNet/sing-box/releases
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

Example VLESS Reality configuration:

```json
{
  "proxies": [
    {
      "name": "Reality-Node-1",
      "enabled": true,
      "local_port": 10801,
      "type": "vless",
      "server": "edge.example.com",
      "port": 443,
      "uuid": "your-vless-uuid",
      "security": "reality",
      "flow": "xtls-rprx-vision",
      "server_name": "www.example.com",
      "fingerprint": "chrome",
      "reality_public_key": "your-reality-public-key",
      "reality_short_id": "0123456789abcdef"
    }
  ]
}
```

See [`configs/config.example.json`](configs/config.example.json) for examples of all supported protocols. VLESS support is intentionally limited to TCP + Reality + XTLS Vision.

To import a subscription, copy its HTTP(S) URL and choose **Import Subscription from Clipboard** from the tray menu.

#### 3. Build

```powershell
# One-command build (requires Go, CMake, MSVC 2022, Qt 6.8.3, and sing-box.exe)
.\build.ps1

# Build the portable files and dist/OneProxy-0.6.0-setup.exe
.\build.ps1 -Installer
```

The script discovers MSVC and Qt through `vswhere.exe` and `qmake.exe`. Custom locations can be passed with `-VcVars`, `-QtDir`, and `-InnoSetup`, or their `ONEPROXY_*` environment variables. Generated DLLs, executables, Qt deployment files, and `.srs` rule sets are intentionally not tracked by Git; the build recreates them from the tracked sources.

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

### SOCKS5 and HTTP CONNECT

Every local port is a sing-box `mixed` inbound and accepts both SOCKS5 and HTTP CONNECT. No protocol switch is required:

```powershell
# Test HTTP proxy
curl -x http://127.0.0.1:10801 https://ip.sb
```

OneProxy intentionally does not change Windows system proxy settings. Configure the local port in each application or browser to avoid interfering with PAC files and other proxy clients.

### Routing Modes

OneProxy supports three routing modes, selectable from the tray menu:

| Mode | Behavior |
|------|----------|
| **Global** | Unified port sends all traffic through the selected proxy (default) |
| **Rule** | Unified port sends China IPs/domains direct and other traffic through the proxy |
| **Direct** | Unified port sends all traffic directly |

Individual node ports always remain pinned to their corresponding upstream node.

Rule mode uses sing-box's built-in `rule_set` router with community-maintained databases:

| Database | Size | Purpose |
|----------|------|---------|
| `geoip-cn.srs` | China IP ranges | China IPs → direct |
| `geosite-cn.srs` | China domain rules | China domains → direct |

The rule sets are loaded from the installation's `bin/` directory and packaged by the installer.

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
| **Start All Proxies** | Start all enabled proxies |
| **Stop All Proxies** | Stop sing-box and close all ports |
| **Restart All Proxies** | Restart and trigger health check |
| **Check All Nodes** | Run an immediate health check |
| **Flush DNS** | Flush system DNS and restart sing-box |
| **Routing Mode** | Select Global, Rule, or Direct mode |
| **Import Subscription** | Import an HTTP(S), SS, VMess, or VLESS source |
| **Quit** | Stop proxies and quit |

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
char* OneProxy_SelectProxy(char* proxyName);
char* OneProxy_ExportConfig();
char* OneProxy_ImportConfig(char* input);
char* OneProxy_GetVersion();
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
**Solution:** Another process is using the port. Identify its owner, then change `local_port` in `config.json` or close the owning application deliberately. Do not terminate processes by name because they may belong to an active proxy session.
```powershell
netstat -ano | findstr :10801
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
type %USERPROFILE%\.oneproxy\logs\singbox.log
```

### Tray icon not showing
- Qt platform plugin missing — ensure `platforms\qwindows.dll` exists in build dir
- Run from PowerShell to see error output:
```powershell
cd trayapp\build
.\oneproxy-tray.exe
# Check console output
```

## Project Structure

```
OneProxy/
├── cmd/
│   ├── oneproxy/          # Go CLI executable
│   └── oneproxy-dll/      # Go DLL (C shared library)
├── internal/
│   ├── config/            # Config parser + sing-box config generator
│   │   ├── config.go      # JSON unmarshal, validation
│   │   ├── subscription.go # SS, VMess, and VLESS subscription parser
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
├── build.ps1              # One-click build script
└── config-placeholder.json # Safe first-run template
```

---

## Development

Run `go test ./...` and `go vet ./...` before building. Windows toolchain paths and the end-to-end verification checklist are documented in [`CLAUDE.md`](CLAUDE.md).

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
├── config-placeholder.json  # Safe first-run template
├── green.ico, yellow.ico, red.ico
├── Qt6Core.dll, Qt6Gui.dll, Qt6Widgets.dll
├── platforms/
│   └── qwindows.dll         # Qt platform plugin
└── bin/
    └── sing-box.exe         # Proxy engine
```

Runtime configuration and logs are stored under `%USERPROFILE%\.oneproxy\`.

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
A: The Go and Qt targets are compile-checked on macOS and Linux, but only Windows is runtime-verified and released. See [`docs/CROSS_PLATFORM.md`](docs/CROSS_PLATFORM.md) for the exact boundary.

**Q: Can I use HTTP proxies instead of SOCKS5?**  
A: Yes. Every OneProxy local port accepts both SOCKS5 and HTTP CONNECT.

**Q: How do I add a new proxy node?**  
A: Edit `config.json`, add a new entry to `proxies` array with a unique `local_port`, restart via tray menu.

**Q: Is this faster than using a VPN?**  
A: SOCKS5 proxies have lower overhead than VPNs (no TUN/TAP layer), but speed depends on your upstream server quality.
