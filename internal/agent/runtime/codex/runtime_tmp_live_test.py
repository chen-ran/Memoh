"""Offline smoke test; run only in a disposable Linux workspace/container.

Usage: python3 -I runtime_tmp_live_test.py /path/to/pinned/codex /data
The parent directory must exist. Only a newly allocated test home is modified.
This does not test OAuth, model calls, NFS remounts or native thread recovery.
"""

import importlib.util
import json
import os
from pathlib import Path
import selectors
import signal
import subprocess
import sys
import tempfile
import time


spec = importlib.util.spec_from_file_location(
    "runtime_tmp", Path(__file__).with_name("runtime_tmp.py")
)
layout = importlib.util.module_from_spec(spec)
spec.loader.exec_module(layout)


def initialize(binary, home, stderr, processes):
    process = subprocess.Popen(
        [binary, "app-server"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=stderr,
        cwd=home,
        env={"CODEX_HOME": str(home), "PATH": "/usr/local/bin:/usr/bin:/bin", "RUST_LOG": "error"},
        start_new_session=True,
    )
    processes.append(process)
    request = {"id": 1, "method": "initialize", "params": {"clientInfo": {"name": "memoh-layout-test", "version": "test"}}}
    process.stdin.write((json.dumps(request) + "\n").encode())
    process.stdin.flush()
    deadline = time.monotonic() + 15
    with selectors.DefaultSelector() as poller:
        poller.register(process.stdout, selectors.EVENT_READ)
        while time.monotonic() < deadline:
            if not poller.select(max(0, deadline - time.monotonic())):
                break
            response = json.loads(process.stdout.readline())
            if response.get("id") == 1:
                if "error" in response:
                    raise AssertionError("initialize returned an error")
                return response["result"]["userAgent"]
    raise AssertionError("initialize timed out")


def main(binary, parent):
    if not Path(binary).is_absolute():
        raise AssertionError("provide an absolute binary path")
    version = subprocess.check_output([binary, "--version"], text=True).strip()
    with tempfile.TemporaryDirectory(prefix="memoh-codex-layout-test-", dir=parent) as name:
        home = Path(name)
        (home / "config.toml").write_text("# smoke-test configuration\n")
        sentinel = home / "sessions" / "preserved.txt"
        sentinel.parent.mkdir()
        sentinel.write_text("native data must remain durable")
        assert layout.prepare(str(home)) == "isolated"
        target = (home / "tmp").resolve()
        processes = []
        started = time.monotonic()
        try:
            with tempfile.TemporaryFile() as stderr:
                first = initialize(binary, home, stderr, processes)
                aliases = list((home / "tmp" / "arg0").glob("*/apply_patch"))
                assert len(aliases) == 1, "helper aliases were skipped"
                old_alias = aliases[0]
                second = initialize(binary, home, stderr, processes)
                assert old_alias.exists(), "second startup removed a live process's helper"
                aliases = list((home / "tmp" / "arg0").glob("*/apply_patch"))
                assert len(aliases) == 2, "processes must own distinct helper directories"
                for index, alias in enumerate(aliases):
                    patch = "*** Begin Patch\n*** Add File: helper-%d.txt\n+helper works\n*** End Patch\n" % index
                    result = subprocess.run(
                        [str(alias), patch], cwd=home, capture_output=True, timeout=10,
                        env={"PATH": "/usr/local/bin:/usr/bin:/bin"},
                    )
                    assert result.returncode == 0, "native apply_patch alias failed"
                    assert (home / f"helper-{index}.txt").read_text() == "helper works\n"
                assert sentinel.read_text() == "native data must remain durable"
                assert target == (home / "tmp").resolve()
                for process in processes:
                    process.stdin.close()
                    assert process.wait(timeout=10) == 0, "app-server did not exit cleanly on EOF"
                assert not list((home / "tmp" / "arg0").glob("*/.lock")), "arg0 guards leaked"
                stderr.seek(0)
                assert b"could not create PATH aliases" not in stderr.read(), "alias setup degraded"
                print(json.dumps({"version": version, "clients": [first, second], "seconds": round(time.monotonic() - started, 3), "helper_aliases": "passed", "durable_sentinel": "preserved", "clean_exit": True}))
        finally:
            # Only terminate children created by this test, never preexisting agents.
            for process in processes:
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)
                for stream in (process.stdin, process.stdout):
                    if stream and not stream.closed:
                        stream.close()


if __name__ == "__main__":
    main(*sys.argv[1:])
