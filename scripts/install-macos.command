#!/bin/bash
set -euo pipefail
if [[ "$EUID" -eq 0 ]]; then
  echo 'Run as your normal user, not with sudo: upgrade must use your tray state directory.' >&2
  exit 1
fi

# Run from the extracted download, not from the installed app. No sudo needed
# for a user-owned destination; a permission error leaves the old app intact.
here="$(cd "$(dirname "$0")" && pwd)"
source_app="$here/oneproxy-tray.app"
destination="${1:-/Applications/oneproxy-tray.app}"
if [[ "$(uname -s)" != Darwin || ! -x "$source_app/Contents/MacOS/oneproxy-tray" ]]; then
  echo 'Keep this script beside the extracted oneproxy-tray.app and run it on macOS.' >&2
  exit 1
fi
echo 'Installing OneProxy. The current connection will stop briefly.'
"$source_app/Contents/MacOS/oneproxy-tray" --install-update "$source_app" "$destination"
echo "Installation complete. Open $destination to reconnect."
