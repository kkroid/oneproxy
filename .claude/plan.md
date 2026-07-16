# v0.6.0 Feature Plan

## 1. System Proxy Toggle

### Config
- `config.go`: add `SystemProxy bool` field to Config

### C++ Tray
- `main.cpp`: add checkable action "系统代理" / "System Proxy" at top of menu
- `i18n.h`: add `systemProxy` string
- On toggle: write `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
  - `ProxyEnable` = 1/0
  - `ProxyServer` = `socks=127.0.0.1:1080`
  - `ProxyOverride` = `<local>`
- Call `InternetSetOption(NULL, INTERNET_OPTION_SETTINGS_CHANGED, ...)` to apply immediately
- On startup, read registry to sync checkbox state
- On quit, disable proxy automatically

### Go DLL  
- No changes needed — pure Windows registry operation, done in C++ tray

## 2. Routing Mode (Direct / Global / Rule)

### Config
- `config.go`: add `RouteMode string` field (`"rule"` default / `"global"` / `"direct"`)

### Go: singbox.go
- `generateRoute()`:
  - `"direct"`: final="direct", no proxy rules
  - `"global"`: final="proxy" (selector)
  - `"rule"`: add geoip:cn→direct rule, geosite:cn→direct rule, final="proxy", plus dns→direct rule
- Need `geoip.db` and `geosite.db` downloaded to `bin/`
  - Download from: https://github.com/SagerNet/sing-geoip/releases / sing-geosite

### C++ Tray
- `main.cpp`: submenu "代理模式" / "Routing Mode" with three radio items
- `i18n.h`: add routing mode strings
- On change: restart proxy to apply new route config

### CLI (oneproxy.exe)
- Display current routing mode in startup output

## 3. Health Check Concurrency

### Already concurrent
- `CheckAll()` already uses goroutines + WaitGroup
- `CheckProxy()` is thread-safe (locks resultsMux)
- **No changes needed** — the 60s 串行印象 was from the tray showing results after a fixed 8s timer, not from the actual check being slow

## Implementation Order

1. Health check concurrency (verify, already done)
2. System proxy toggle
3. Routing mode

## Files Changed

| # | File | Change |
|---|------|--------|
| 1 | `internal/config/config.go` | Add `SystemProxy`, `RouteMode` fields |
| 2 | `internal/config/singbox.go` | Modify `generateRoute()` for routing mode |
| 3 | `trayapp/i18n.h` | Add system proxy + routing mode strings |
| 4 | `trayapp/main.cpp` | System proxy toggle + routing mode submenu |
| 5 | `CLAUDE.md` | Update |

## Verification

1. `go build -buildmode=c-shared` + `go vet` zero errors
2. DLL Start → curl test → Stop
3. Build tray → verify menu shows new items
4. System proxy toggle → verify `reg query` shows correct values
5. Routing mode change → verify generated sing-box config
