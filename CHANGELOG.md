# Changelog

## 0.6.0 - Unreleased

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
