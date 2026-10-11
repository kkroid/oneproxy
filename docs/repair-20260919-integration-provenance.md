# Integration provenance

Task: `repair-20260919-integration`, recovery round 2. Integration base:
`2c1cd5b6471c047aa6fbffe62533ee19f9daa17c`.
The recovery instruction authorizes only the necessary ancestor closure of the
six independently reviewed task ranges. This document does not grant PASS to
the new integration; it is submitted for independent review.

## Preserved tip provenance

| Source task | Reviewed tip | Round-1 integration commit |
| --- | --- | --- |
| windows-trust-1-dns | 3a46d942488578f8c5fc0d3c1955789a5d22a333 | 67bc5b0829a4c2ad23ef0c41e196d17975bf6489 |
| windows-trust-2-startup | 95f9bbf7e6367135144dc110f13ff4b667851981 | f6dd14c79518a42b82adaceec272aa155c1029dd |
| windows-trust-3-autostart | 8a6f35f197179d51eabbd13f38a19318424b50ff | 0a9ac063996dc72ac3369d9147629821f3c44c8d |
| windows-trust-5-metadata | e370b8637de34ab2805642bb26c5ff6d4dc8637e | f0a6024075909021137e91c3430cbf330cf6b84a |
| windows-trust-4-signing | 5a404c35482e02c17676de70d8560635cd906f44 | fe3ef1a35ec3720d26def4a944a54db3897c943e |
| repair-20260919-C | a13b3ae87a650dcb1fe7166cd53327c844eaa53a | 3cd0922f12c182d4d5646f724ec515dd9da9b472 |

Official implementation PASS evidence, relative to run
`oneproxy-ubuntu2404-cli-20260907-01`:

- `windows-trust-1-dns/impl-audit/r1-a0-w0-dispatch/windows-trust-1-dns-impl-audit-r1-result.json`
- `windows-trust-2-startup/impl-audit/r2-a0-w0-recovery/windows-trust-2-startup-impl-audit-r2-result.json`
- `windows-trust-3-autostart/impl-audit/r1-a0-w0-dispatch/windows-trust-3-autostart-impl-audit-r1-result.json`
- `windows-trust-5-metadata/impl-audit/r2-a0-w0-recovery/windows-trust-5-metadata-impl-audit-r2-result.json`
- `windows-trust-4-signing/impl-audit/r4-a0-w0-recovery/windows-trust-4-signing-impl-audit-r4-result.json`
- `repair-20260919-C/impl-audit/r3-a0-w0-recovery/repair-20260919-C-impl-audit-r3-result.json`

## Necessary reviewed ancestors

Startup, autostart, metadata and signing review base is the integration base.
DNS review base is startup tip `95f9bbf7e6367135144dc110f13ff4b667851981`.
Repair C review base is `50596cc3644a7546dabf7dd83987ba61a9acacc1`.

| Source | Ancestor | Necessary contract |
| --- | --- | --- |
| startup | f55041816361910a4eedbacad87b2764c4bd333f | Manager readiness, observable process exit and serialized lifecycle required by DNS tests |
| startup | cd8103ef6b02d105097453d37a2ddbc2c8b24410 | DLL lifecycle error propagation and state handling required by startup tip |
| DNS | 70ccf741d98f9f7cdcba0803f31e790a2773e933 | Explicit manual recovery; all-down is not DNS evidence |
| DNS | 716a789779498ae797f92aedb0426a2096ee3b1b | Windows DNS API success/failure handling |
| DNS | d3e4309bc881dac45c7a8ed18cfe4af49365c4ac | Serialized recovery and failure/cooldown contracts consumed by tip tests |
| DNS | 528c43cbf792aed0c5ec8cd6d4bad027453576ea | Merge of reviewed startup readiness into DNS, not an extra feature |
| metadata | 5fb80f647202a9831e9f665877566d075760a68c | Version resource generation, linker version injection and release manifest prerequisites |
| signing | 1cf739963ffecf4aee0573a579f9fab7ce639255 | Explicit opt-in Windows signing pipeline |
| signing | 085e4c0a76ecbef72cb74fce07dc4878244c535f | Signing command safety and pre-package verification refinements |
| signing | 9177e1cbc41e301d7df07bb395e74002df766791 | Inno signing integration prerequisites |
| signing | 9a86f1067baf0575e839207d4789d78f2dceaa5f | Strict release/signature verification prerequisites |
| repair C | c60186c3c3c0d55a1daf2fe610861098739a01de | Bundled CLI/runtime sync, DX dependencies, platform resource self-check and installer platform constraints |

`f550418` was cherry-picked with `-x` as
`738ff670b7a132bf87818ffd2b6476e3f89761a6`.
The other prerequisites are integrated from the reviewed final source snapshots
or narrowly combined in overlapping files, rather than replaying already
integrated tips. DNS production files and tests match its reviewed tip, except
the DLL version variable required by metadata. Metadata templates and signing
documentation are restored from their reviewed tips. Shared build/installer
paths combine the reviewed metadata, signing and repair C behaviors.

Repair C's `.github` changes are outside writeScope and excluded. Its older
0.8.0 baseline must not replace 0.8.1, InstanceGuard, legacy process protection,
installer upgrade checks or ownership-aware autostart. Those newer behaviors
are retained. The release manifest payload filter and fixture additionally
include the reviewed repair C CLI, dxcompiler.dll and dxil.dll; compiler probes,
build helpers and user configuration remain excluded.

## Validation boundaries

Recovery combination: `go test ./...`, `go vet ./...`, and
`go test -race ./...` passed. PowerShell build syntax parsed successfully.
Build and artifact evidence is published with the round-2 result outside the
source tree, so a committed build can report `sourceDirty=false`.

The existing local dependency is copied only from
`C:/Users/kkroid/Documents/OneProxy/bin/sing-box.exe`, SHA256
`DB0D779948214CF761011D154C3A5DA36DF20394FA01A9FC798F1DC39FE9D183`.
No real configuration is read or copied. No product is installed or started;
the tray self-check loads the DLL/resources without starting the proxy.
Baseline live processes: oneproxy-tray PID 33728 (2026-10-07 16:09:43),
sing-box PID 31524 (2026-10-07 16:09:45); final read-only comparison is in the
result evidence.

Installer compilation is distinct from actual installation, public-trust
signing, successful online RFC3161 timestamps and execution on a pristine
Windows machine. None of those claims follow from an unsigned build.
No macOS build, CI or ZIP runtime execution is performed in this recovery.
