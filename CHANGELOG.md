# Changelog

## 0.7.0 - 2026-08-11

### Changed

- Upgraded the Go build baseline to 1.26 and added native Windows, macOS, and Linux compile checks for the CLI, shared library, and Qt tray application. Windows remains the only runtime-verified and released platform.
- macOS CI now builds an unsigned `.app` bundle zip with Qt, the platform shared library, sing-box, and compiled rule sets.

## 0.6.0 - 2026-08-03

### Added

- VLESS subscription and outbound support for TCP + Reality + XTLS Vision.
- Direct `vless://` clipboard imports and `VL` protocol labels in the tray.
- Regression tests for VLESS parsing, mixed subscriptions, config validation, and sing-box Reality output.

### Changed

- Local ports are documented as mixed SOCKS5 + HTTP CONNECT inbounds.
- Unknown proxy types are rejected during configuration validation.
- English and Chinese documentation now match the runtime paths, routing rule-set files, and current DLL API.
- `build.ps1 -Installer` now builds the setup package and no longer terminates running proxy processes.
- Windows builds now discover their toolchains, compile rule sets from tracked JSON sources, and reproduce the installer in GitHub Actions.

### Removed

- The Windows system-proxy toggle, including its registry writes and WinINet dependency. The previous implementation used a hard-coded port and could conflict with PAC or proxy settings owned by other applications.
- Generated binaries, Qt deployment files, and compiled `.srs` rule sets from version control.

### Compatibility

- Shadowsocks and VMess configurations remain supported.
- VLESS support is intentionally limited to the profile listed above; WebSocket, gRPC, and non-Reality variants are rejected.
