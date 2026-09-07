#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
real_sing_box="${SING_BOX_BIN:-}"
if [[ -z "$real_sing_box" || ! -x "$real_sing_box" ]]; then
  echo "SING_BOX_BIN must name an executable sing-box 1.13.14 binary" >&2
  exit 1
fi

work="$(mktemp -d)"
declare -a cleanup_pids=()
cleanup() {
  local pid
  for pid in "${cleanup_pids[@]:-}"; do
    kill -TERM "$pid" 2>/dev/null || true
    kill -KILL "$pid" 2>/dev/null || true
  done
  rm -rf -- "$work"
}
trap cleanup EXIT

fail() {
  echo "ubuntu CLI verification failed: $*" >&2
  exit 1
}

expect_exit() {
  local want="$1"
  shift
  set +e
  "$@"
  local got=$?
  set -e
  [[ "$got" -eq "$want" ]] || fail "$* exited $got, want $want"
}

free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(('127.0.0.1', 0))
print(s.getsockname()[1])
s.close()
PY
}

write_config() {
  local path="$1"
  local port="$2"
  cat >"$path" <<JSON
{
  "version": "1.0",
  "log_level": "error",
  "route_mode": "direct",
  "unified": {"port": 0},
  "health_check": {"enabled": false, "interval_seconds": 0, "timeout_seconds": 1, "test_url": "http://127.0.0.1/"},
  "dns": {"flush_on_failure": false, "flush_interval_seconds": 0, "servers": []},
  "proxies": [{
    "name": "local-test",
    "enabled": true,
    "local_port": $port,
    "type": "shadowsocks",
    "server": "127.0.0.1",
    "port": 9,
    "method": "aes-128-gcm",
    "password": "verification-secret"
  }],
  "inbound": {"listen": "127.0.0.1"}
}
JSON
  chmod 600 "$path"
}

wait_for_running() {
  local pid="$1"
  local output="$2"
  local attempt
  for attempt in $(seq 1 60); do
    grep -q "OneProxy running" "$output" && return 0
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  fail "OneProxy did not reach the running milestone; output=$(tr '\n' ' ' <"$output")"
}

assert_no_group() {
  local pgid="$1"
  if ps -eo pgid= | awk -v wanted="$pgid" '$1 == wanted { found=1 } END { exit !found }'; then
    fail "process group $pgid still has members"
  fi
}

install_dir="$work/install"
home_dir="$work/home"
mkdir -p "$install_dir/bin" "$home_dir"
cp "$real_sing_box" "$install_dir/bin/sing-box"
chmod 755 "$install_dir/bin/sing-box"
if [[ -n "${ONEPROXY_BIN:-}" ]]; then
  cp "$ONEPROXY_BIN" "$install_dir/oneproxy"
else
  (
    cd "$repo_root"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -o "$install_dir/oneproxy" ./cmd/oneproxy
  )
fi
chmod 755 "$install_dir/oneproxy"

help_out="$work/help.out"
HOME="$work/unused-home" "$install_dir/oneproxy" --help >"$help_out"
grep -q "oneproxy --config <path>" "$help_out" || fail "--help did not document --config"
expect_exit 2 "$install_dir/oneproxy" >"$work/missing.out" 2>&1
expect_exit 2 "$install_dir/oneproxy" --config >"$work/value.out" 2>&1
expect_exit 2 "$install_dir/oneproxy" --unknown >"$work/unknown.out" 2>&1
expect_exit 2 "$install_dir/oneproxy" start >"$work/positional.out" 2>&1

malformed="$work/malformed.json"
printf '{' >"$malformed"
chmod 600 "$malformed"
expect_exit 1 env HOME="$home_dir" "$install_dir/oneproxy" --config "$malformed" >"$work/malformed.out" 2>&1
! grep -q "OneProxy running" "$work/malformed.out" || fail "malformed config printed a running milestone"

private_config="$work/private.json"
write_config "$private_config" "$(free_port)"
chmod 0644 "$private_config"
before_hash="$(sha256sum "$private_config")"
expect_exit 1 env HOME="$home_dir" "$install_dir/oneproxy" --config "$private_config" >"$work/permissions.out" 2>&1
grep -q "chmod 600" "$work/permissions.out" || fail "permission error lacks chmod 600 guidance"
[[ "$(sha256sum "$private_config")" == "$before_hash" ]] || fail "input config was modified"
[[ "$(stat -c %a "$private_config")" == "644" ]] || fail "input config mode was modified"
chmod 600 "$private_config"

occupied_port="$(free_port)"
occupied_ready="$work/occupied.ready"
python3 - "$occupied_port" "$occupied_ready" <<'PY' &
import socket, sys, time
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('127.0.0.1', int(sys.argv[1])))
s.listen()
open(sys.argv[2], 'w').close()
while True:
    time.sleep(1)
PY
holder_pid=$!
cleanup_pids+=("$holder_pid")
holder_index=$((${#cleanup_pids[@]} - 1))
for _ in $(seq 1 30); do [[ -e "$occupied_ready" ]] && break; sleep 0.1; done
[[ -e "$occupied_ready" ]] || fail "port holder did not start"
occupied_config="$work/occupied.json"
write_config "$occupied_config" "$occupied_port"
expect_exit 1 env HOME="$home_dir" "$install_dir/oneproxy" --config "$occupied_config" >"$work/occupied.out" 2>&1
! grep -q "OneProxy running" "$work/occupied.out" || fail "immediate child failure printed a running milestone"
kill -TERM "$holder_pid"
wait "$holder_pid" 2>/dev/null || true
cleanup_pids[$holder_index]=""

run_port="$(free_port)"
run_config="$work/run.json"
write_config "$run_config" "$run_port"
mkdir -p "$home_dir/.oneproxy/logs"
chmod 0755 "$home_dir/.oneproxy" "$home_dir/.oneproxy/logs"
printf old >"$home_dir/.oneproxy/singbox_generated.json"
printf old >"$home_dir/.oneproxy/logs/singbox.log"
chmod 0644 "$home_dir/.oneproxy/singbox_generated.json" "$home_dir/.oneproxy/logs/singbox.log"

run_output="$work/run.out"
HOME="$home_dir" "$install_dir/oneproxy" --config "$run_config" >"$run_output" 2>&1 &
parent_pid=$!
cleanup_pids+=("$parent_pid")
parent_index=$((${#cleanup_pids[@]} - 1))
wait_for_running "$parent_pid" "$run_output"
child_pid="$(pgrep -P "$parent_pid" sing-box | head -n1)"
[[ -n "$child_pid" ]] || fail "sing-box child was not found"
child_pgid="$(ps -o pgid= -p "$child_pid" | tr -d ' ')"
sleep 2
kill -0 "$parent_pid" && kill -0 "$child_pid" || fail "foreground processes did not sustain"
[[ "$(stat -c %a "$home_dir/.oneproxy")" == "700" ]] || fail "state directory is not 0700"
[[ "$(stat -c %a "$home_dir/.oneproxy/logs")" == "700" ]] || fail "log directory is not 0700"
[[ "$(stat -c %a "$home_dir/.oneproxy/singbox_generated.json")" == "600" ]] || fail "generated config is not 0600"
[[ "$(stat -c %a "$home_dir/.oneproxy/logs/singbox.log")" == "600" ]] || fail "sing-box log is not 0600"
! grep -q "verification-secret" "$run_output" || fail "credential appeared in normal output"
kill -TERM "$parent_pid"
set +e
wait "$parent_pid"
run_status=$?
set -e
[[ "$run_status" -eq 0 ]] || fail "SIGTERM shutdown exited $run_status"
cleanup_pids[$parent_index]=""
kill -0 "$child_pid" 2>/dev/null && fail "sing-box child survived SIGTERM shutdown"
assert_no_group "$child_pgid"
python3 - "$run_port" <<'PY' || fail "test inbound remained open after shutdown"
import socket, sys
s = socket.socket()
s.settimeout(0.2)
if s.connect_ex(('127.0.0.1', int(sys.argv[1]))) == 0:
    raise SystemExit(1)
PY

late_port="$(free_port)"
late_config="$work/late.json"
write_config "$late_config" "$late_port"
late_output="$work/late.out"
HOME="$home_dir" "$install_dir/oneproxy" --config "$late_config" >"$late_output" 2>&1 &
late_parent=$!
cleanup_pids+=("$late_parent")
late_index=$((${#cleanup_pids[@]} - 1))
wait_for_running "$late_parent" "$late_output"
late_child="$(pgrep -P "$late_parent" sing-box | head -n1)"
[[ -n "$late_child" ]] || fail "late-exit child was not found"
kill -KILL "$late_child"
set +e
wait "$late_parent"
late_status=$?
set -e
[[ "$late_status" -eq 1 ]] || fail "unexpected child exit returned $late_status, want 1"
cleanup_pids[$late_index]=""
sleep 0.2
kill -0 "$late_child" 2>/dev/null && fail "foreground mode left or restarted sing-box"

force_dir="$work/force-install"
force_home="$work/force-home"
mkdir -p "$force_dir/bin" "$force_home"
cp "$install_dir/oneproxy" "$force_dir/oneproxy"
cat >"$force_dir/bin/sing-box" <<'SH'
#!/usr/bin/env bash
if [[ "${1:-}" == "check" ]]; then
  exec "$ONEPROXY_REAL_SING_BOX" "$@"
fi
trap '' TERM
while :; do sleep 10; done
SH
chmod 755 "$force_dir/bin/sing-box"
force_config="$work/force.json"
write_config "$force_config" "$(free_port)"
force_output="$work/force.out"
ONEPROXY_REAL_SING_BOX="$real_sing_box" HOME="$force_home" "$force_dir/oneproxy" --config "$force_config" >"$force_output" 2>&1 &
force_parent=$!
cleanup_pids+=("$force_parent")
force_index=$((${#cleanup_pids[@]} - 1))
wait_for_running "$force_parent" "$force_output"
force_child="$(pgrep -P "$force_parent" | head -n1)"
[[ -n "$force_child" ]] || fail "forced-kill child was not found"
force_pgid="$(ps -o pgid= -p "$force_child" | tr -d ' ')"
force_started="$(date +%s)"
kill -TERM "$force_parent"
set +e
wait "$force_parent"
force_status=$?
set -e
force_elapsed=$(( $(date +%s) - force_started ))
[[ "$force_status" -eq 0 ]] || fail "forced-kill shutdown exited $force_status"
cleanup_pids[$force_index]=""
[[ "$force_elapsed" -ge 5 ]] || fail "SIGKILL fallback occurred before the five-second grace period"
assert_no_group "$force_pgid"

echo "Ubuntu foreground CLI verification passed"
