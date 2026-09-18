#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/build-macos.sh [options]

Options:
  --arch ARCH       x86_64, arm64, or universal (default: host architecture)
  --qt-root PATH    Qt installation root (default: $QT_ROOT_DIR or $HOME/Qt)
  --skip-tests      Skip Go tests, vet, and runnable tray tests
  -h, --help        Show this help
EOF
}

arch=''
qt_root="${QT_ROOT_DIR:-$HOME/Qt}"
skip_tests=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --arch)
      [[ $# -ge 2 ]] || { echo '--arch requires a value' >&2; exit 2; }
      arch="$2"
      shift 2
      ;;
    --qt-root)
      [[ $# -ge 2 ]] || { echo '--qt-root requires a path' >&2; exit 2; }
      qt_root="$2"
      shift 2
      ;;
    --skip-tests)
      skip_tests=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

host_arch="$(uname -m)"
case "$host_arch" in
  x86_64|arm64) ;;
  *) echo "Unsupported macOS host architecture: $host_arch" >&2; exit 1 ;;
esac

if [[ -z "$arch" ]]; then
  arch="$host_arch"
fi
case "$arch" in
  x86_64|arm64|universal) ;;
  *) echo "Unsupported target architecture: $arch" >&2; exit 2 ;;
esac

if [[ "$(uname -s)" != Darwin ]]; then
  echo 'This script requires macOS with Xcode command line tools.' >&2
  exit 1
fi

if [[ -n "${VIRTUAL_ENV:-}" ]]; then
  export PATH="$VIRTUAL_ENV/bin:$PATH"
fi

qt_root="$(cd "$qt_root" && pwd)"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

for command_name in go cmake clang lipo curl tar shasum file codesign ditto python3; do
  command -v "$command_name" >/dev/null || {
    echo "Required command not found: $command_name" >&2
    exit 1
  }
done

macdeployqt="$qt_root/bin/macdeployqt"
if [[ ! -x "$macdeployqt" ]]; then
  echo "macdeployqt not found: $macdeployqt" >&2
  exit 1
fi

case "$host_arch" in
  x86_64) host_singbox_arch='amd64' ;;
  arm64) host_singbox_arch='arm64' ;;
esac

target_arches=()
if [[ "$arch" == universal ]]; then
  target_arches=(x86_64 arm64)
else
  target_arches=("$arch")
fi

for target_arch in "${target_arches[@]}"; do
  lipo "$qt_root/lib/QtCore.framework/QtCore" -verify_arch "$target_arch"
done

go_arch_for() {
  [[ "$1" == x86_64 ]] && echo amd64 || echo arm64
}

output_dir="$root/dist/macos-$arch"
build_root="$root/trayapp/build-macos-$arch"
mkdir -p "$output_dir"

if [[ "$skip_tests" -eq 0 ]]; then
  GOOS=darwin GOARCH="$(go_arch_for "$host_arch")" CGO_ENABLED=1 \
    CC="clang -arch $host_arch" go test -count=1 ./...
  GOOS=darwin GOARCH="$(go_arch_for "$host_arch")" CGO_ENABLED=1 \
    CC="clang -arch $host_arch" go vet ./...
fi

for target_arch in "${target_arches[@]}"; do
  go_arch="$(go_arch_for "$target_arch")"
  target_dir="$root/dist/.macos-$arch/$target_arch"
  mkdir -p "$target_dir"

  GOOS=darwin GOARCH="$go_arch" CGO_ENABLED=0 \
    go build -buildvcs=false -o "$target_dir/oneproxy" ./cmd/oneproxy

  GOOS=darwin GOARCH="$go_arch" CGO_ENABLED=1 \
    CC="clang -arch $target_arch" \
    go build -buildvcs=false -buildmode=c-shared \
    -o "$target_dir/liboneproxy.dylib" ./cmd/oneproxy-dll
done

if [[ "$arch" == universal ]]; then
  lipo -create \
    "$root/dist/.macos-universal/x86_64/oneproxy" \
    "$root/dist/.macos-universal/arm64/oneproxy" \
    -output "$output_dir/oneproxy"
  lipo -create \
    "$root/dist/.macos-universal/x86_64/liboneproxy.dylib" \
    "$root/dist/.macos-universal/arm64/liboneproxy.dylib" \
    -output "$output_dir/liboneproxy.dylib"
else
  cp "$root/dist/.macos-$arch/$arch/oneproxy" "$output_dir/oneproxy"
  cp "$root/dist/.macos-$arch/$arch/liboneproxy.dylib" "$output_dir/liboneproxy.dylib"
fi

cmake_architectures="$arch"
if [[ "$arch" == universal ]]; then
  cmake_architectures='x86_64;arm64'
fi

build_testing=OFF
if [[ "$skip_tests" -eq 0 && ( "$arch" == universal || "$arch" == "$host_arch" ) ]]; then
  build_testing=ON
fi

cmake -S trayapp -B "$build_root" \
  -DCMAKE_PREFIX_PATH="$qt_root" \
  -DCMAKE_OSX_ARCHITECTURES="$cmake_architectures" \
  -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_TESTING="$build_testing" \
  -DONEPROXY_LIBRARY_PATH="$output_dir/liboneproxy.dylib"
cmake --build "$build_root" --parallel 2

if [[ "$build_testing" == ON ]]; then
  QT_QPA_PLATFORM=offscreen ctest --test-dir "$build_root" --output-on-failure
elif [[ "$skip_tests" -eq 0 ]]; then
  echo "Skipping tray runtime tests: target $arch cannot run on host $host_arch"
fi

app="$build_root/oneproxy-tray.app"
app_dir="$app/Contents/MacOS"
resources_dir="$app/Contents/Resources"
mkdir -p "$resources_dir/bin"

download_singbox() {
  local singbox_arch="$1"
  local expected archive archive_path extract_dir
  case "$singbox_arch" in
    amd64) expected='5245d645e847f90bb708da74bc020ae078c28489690756419685c04f56b4e3bb' ;;
    arm64) expected='73e8967b0fc08e17bce4263ca56ebc394822401a16497a1c4e02316c888202ab' ;;
    *) echo "Unsupported sing-box architecture: $singbox_arch" >&2; return 1 ;;
  esac
  archive="sing-box-1.13.14-darwin-${singbox_arch}.tar.gz"
  archive_path="$tmp/$archive"
  extract_dir="$tmp/${singbox_arch}"
  downloaded_singbox="$extract_dir/sing-box-1.13.14-darwin-${singbox_arch}/sing-box"
  [[ ! -x "$downloaded_singbox" ]] || return 0
  curl --retry 3 --fail --location --silent --show-error \
    "https://github.com/SagerNet/sing-box/releases/download/v1.13.14/$archive" \
    -o "$archive_path"
  echo "$expected  $archive_path" | shasum -a 256 -c - >/dev/null
  mkdir -p "$extract_dir"
  tar -xzf "$archive_path" -C "$extract_dir"
  [[ -x "$downloaded_singbox" ]] || { echo "sing-box not found in $archive" >&2; return 1; }
}

tmp="$(mktemp -d "${TMPDIR:-/tmp}/oneproxy-macos.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

download_singbox "$host_singbox_arch"
host_singbox="$downloaded_singbox"
if [[ "$arch" == universal ]]; then
  download_singbox arm64
  arm_singbox="$downloaded_singbox"
  download_singbox amd64
  intel_singbox="$downloaded_singbox"
  lipo -create "$intel_singbox" "$arm_singbox" -output "$app_dir/sing-box"
else
  target_singbox_arch="$(go_arch_for "$arch")"
  download_singbox "$target_singbox_arch"
  cp "$downloaded_singbox" "$app_dir/sing-box"
fi
chmod 755 "$app_dir/sing-box"

cp config-placeholder.json "$resources_dir/config-placeholder.json"
"$host_singbox" rule-set compile bin/geoip-cn.json -o "$resources_dir/bin/geoip-cn.srs"
"$host_singbox" rule-set compile bin/geosite-cn.json -o "$resources_dir/bin/geosite-cn.srs"

"$macdeployqt" "$app" -always-overwrite

while IFS= read -r -d '' binary; do
  if file -b "$binary" | grep -q 'Mach-O'; then
    lipo "$binary" -verify_arch "${target_arches[@]}"
    # The main executable seals the app; sign it last via the bundle path.
    [[ "$binary" == "$app_dir/oneproxy-tray" ]] && continue
    codesign --force --sign - --timestamp=none "$binary"
  fi
done < <(find "$app/Contents" -type f -print0)
while IFS= read -r -d '' bundle; do
  codesign --force --sign - --timestamp=none "$bundle"
done < <(find "$app/Contents" -depth -type d \( -name '*.framework' -o -name '*.app' -o -name '*.xpc' \) -print0)
codesign --force --sign - --timestamp=none "$app"
codesign --verify --deep --strict --verbose=2 "$app"
lipo "$output_dir/oneproxy" -verify_arch "${target_arches[@]}"
codesign --force --sign - --timestamp=none "$output_dir/oneproxy"

ditto -c -k --keepParent "$app" "$root/dist/OneProxy-macos-$arch.zip"

# Verify the distributed app after a ZIP round trip, without starting a proxy.
ditto -x -k "$root/dist/OneProxy-macos-$arch.zip" "$tmp/verify"
verified_app="$tmp/verify/oneproxy-tray.app"
codesign --verify --deep --strict --verbose=2 "$verified_app"
if [[ "$arch" == universal || "$arch" == "$host_arch" ]]; then
  "$verified_app/Contents/MacOS/sing-box" version
  python3 - "$verified_app/Contents/MacOS/liboneproxy.dylib" <<'PY'
import ctypes
import sys
library = ctypes.CDLL(sys.argv[1])
library.OneProxy_GetVersion.restype = ctypes.c_void_p
library.OneProxy_FreeString.argtypes = [ctypes.c_void_p]
version = library.OneProxy_GetVersion()
assert version, 'OneProxy_GetVersion returned NULL'
try:
    print('Packaged Go library version:', ctypes.string_at(version).decode())
finally:
    library.OneProxy_FreeString(version)
PY
else
  echo "Cross-compiled package verified; runtime checks require a $arch Mac."
fi

echo "Built: $app"
echo "Archive: $root/dist/OneProxy-macos-$arch.zip"
file "$output_dir/oneproxy" "$output_dir/liboneproxy.dylib" \
  "$app_dir/oneproxy-tray" "$app_dir/sing-box"
