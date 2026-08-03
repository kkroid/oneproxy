# v0.6.0 Release Preparation

No tag is created by this plan. The goal is a clean, reproducible release candidate.

## Scope

1. Add the deliberately narrow VLESS client profile used by the current upstream:
   - TCP transport
   - Reality security
   - XTLS Vision flow
   - uTLS fingerprint
2. Preserve Shadowsocks and VMess compatibility.
3. Remove the unsafe Windows system-proxy toggle.
4. Keep every local inbound in sing-box `mixed` mode (SOCKS5 + HTTP CONNECT).

## Release Checklist

- [x] Parse VLESS subscription URLs without replacing the URI server with the display label.
- [x] Generate sing-box Reality/uTLS outbound configuration.
- [x] Support direct `vless://` clipboard imports.
- [x] Add parser, mixed-subscription, validation, and generator tests.
- [x] Remove system-proxy UI, registry writes, and WinINet dependency.
- [x] Align English and Chinese README files and example configuration.
- [x] Set runtime and installer version to 0.6.0.
- [x] Make `build.ps1 -Installer` safe for running proxies and functional for packaging.
- [x] Run `go test ./...` and `go vet ./...`.
- [x] Build and load the C-shared DLL in an isolated directory.
- [x] Build the tray application without stopping the installed proxy.
- [x] Generate and hash `dist/OneProxy-0.6.0-setup.exe`.
- [x] Review the final diff and confirm no credentials are tracked.

## Deferred

- General VLESS transports such as WebSocket, gRPC, and ordinary TLS.
- Windows system-proxy ownership, backup, and crash-recovery state management.
- Creating or pushing the `v0.6.0` tag.
