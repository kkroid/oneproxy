"""Isolated real tray/DLL/core lifecycle checks on the macOS CI runner."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def processes():
    rows = subprocess.check_output(["ps", "-axo", "pid=,ppid=,comm="], text=True)
    return {int(p): (int(pp), cmd) for p, pp, cmd in
            (line.strip().split(None, 2) for line in rows.splitlines() if line.strip())}


def wait_until(check, description, timeout=20):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        result = check()
        if result:
            return result
        time.sleep(0.05)
    raise AssertionError("Timed out: " + description)


def run_case(package, driver, mode, report_dir):
    with tempfile.TemporaryDirectory(prefix="oneproxy-quit-") as tmp:
        tmp = Path(tmp)
        app = tmp / "oneproxy-tray.app"
        subprocess.run(["ditto", str(package), str(app)], check=True)
        binary_dir = app / "Contents/MacOS"
        if mode == "menu":
            shutil.copy2(driver, binary_dir / "oneproxy-tray")
            subprocess.run(["codesign", "--force", "--sign", "-", str(app)], check=True)
        home = tmp / "home"
        state = home / ".oneproxy"
        state.mkdir(parents=True)
        with socket.socket() as s:
            s.bind(("127.0.0.1", 0))
            port = s.getsockname()[1]
        config = {
            "version": "1.0", "log_level": "error", "route_mode": "direct",
            "unified": {"port": 0}, "inbound": {"listen": "127.0.0.1"},
            "health_check": {"enabled": False, "timeout_seconds": 1},
            "dns": {"flush_on_failure": False, "flush_interval_seconds": 0, "servers": []},
            "proxies": [{"name": "isolated", "enabled": True, "local_port": port,
                         "type": "shadowsocks", "server": "127.0.0.1", "port": 9,
                         "method": "aes-128-gcm", "password": "fixture-only"}],
        }
        (tmp / "config.json").write_text(json.dumps(config))
        trigger = tmp / "quit.trigger"
        env = dict(os.environ, HOME=str(home), ONEPROXY_TEST_QUIT_FILE=str(trigger),
                   QT_QPA_PLATFORM="offscreen")
        env["QT_QPA_PLATFORM_PLUGIN_PATH"] = os.environ["QT_ROOT_DIR"] + "/plugins/platforms"
        log_path = report_dir / (mode + ".log")
        sentinel = subprocess.Popen(["/bin/sleep", "120"])
        owned = set()
        with log_path.open("w") as log:
            tray = subprocess.Popen([str(binary_dir / "oneproxy-tray")], cwd=tmp,
                                    env=env, stdout=log, stderr=log)
            try:
                def tree_ready():
                    assert tray.poll() is None, "Tray exited during startup"
                    table = processes()
                    supervisors = [p for p, (pp, cmd) in table.items()
                                   if pp == tray.pid and Path(cmd).name == "oneproxy"]
                    owned.update(supervisors)
                    for supervisor in supervisors:
                        cores = [p for p, (pp, cmd) in table.items()
                                 if pp == supervisor and Path(cmd).name == "sing-box"]
                        owned.update(cores)
                        if cores:
                            return supervisor, cores[0]
                supervisor, core = wait_until(tree_ready, "real tray -> supervisor -> sing-box")
                assert (state / "oneproxy-tray.lock").exists(), "HOME was not isolated"
                # Wait beyond Manager's startup window and confirm its real listener.
                time.sleep(1.5)
                with socket.create_connection(("127.0.0.1", port), timeout=2):
                    pass
                assert tray.poll() is None
                before = {"tray": tray.pid, "supervisor": supervisor, "core": core}
                start = time.monotonic()
                if mode == "menu":
                    trigger.touch()  # Driver calls the unmodified production doQuit.
                elif mode == "owner-crash":
                    tray.kill()  # Exact isolated tray PID; never name-based killing.
                else:
                    endpoint = "/tmp/oneproxy-upgrade-" + hashlib.sha256(
                        str(state.resolve()).encode()).hexdigest()
                    with socket.socket(socket.AF_UNIX) as ipc:
                        ipc.settimeout(15)
                        ipc.connect(endpoint)
                        ipc.sendall(b"quit-for-upgrade\n")
                        reply = b""
                        while not reply.endswith(b"\n"):
                            chunk = ipc.recv(64)
                            if not chunk:
                                break
                            reply += chunk
                        assert reply == b"stopped\n", reply
                code = tray.wait(timeout=15)
                assert code == ( -signal.SIGKILL if mode == "owner-crash" else 0), code
                wait_until(lambda: not (set(before.values()) & processes().keys()),
                           "all three recorded processes disappear", timeout=10)
                assert sentinel.poll() is None, "Unrelated sentinel was affected"
                if mode == "menu":
                    assert "TEST: invoking production doQuit" in log_path.read_text()
                # OS must let us reuse the port after cleanup.
                with socket.socket() as s:
                    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                    s.bind(("127.0.0.1", port))
                result = {"mode": mode, "pids": before, "allGone": True,
                          "elapsedSeconds": round(time.monotonic() - start, 3),
                          "portReleased": True, "unrelatedProcessSurvived": True,
                          "traySHA256": sha(binary_dir / "oneproxy-tray"),
                          "dylibSHA256": sha(binary_dir / "liboneproxy.dylib"),
                          "coreSHA256": sha(binary_dir / "sing-box")}
                print(json.dumps(result), flush=True)
                return result
            finally:
                # Only exact fixture identities still in this recorded tree.
                table = processes()
                for pid in owned:
                    if pid in table and (table[pid][0] in {tray.pid, supervisor if 'supervisor' in locals() else 0}
                                         or str(app) in table[pid][1]):
                        try:
                            os.kill(pid, signal.SIGKILL)
                        except ProcessLookupError:
                            pass
                if tray.poll() is None:
                    tray.kill()
                tray.wait()
                sentinel.terminate()
                sentinel.wait()


if __name__ == "__main__":
    assert sys.platform == "darwin", "macOS runner required"
    package, driver, report_dir = map(Path, sys.argv[1:4])
    report_dir.mkdir(parents=True, exist_ok=True)
    results = []
    for mode in ("menu", "packaged-stop", "owner-crash"):
        results.append(run_case(package.resolve(), driver.resolve(), mode, report_dir))
        (report_dir / "results.json").write_text(json.dumps(results, indent=2))
