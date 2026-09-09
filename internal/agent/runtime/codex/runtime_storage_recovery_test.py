"""Disposable NFS/local-volume recovery test using a loopback Responses fixture.

Run `create`, then `migrate`, then `verify` in a replacement workspace with the
same /data and native volume. NLM can be blocked during migrate/verify. No real
model, OAuth credentials, or external network is used. Each command requires
/results for its report and /data/.codex/agents/recovery-test to be disposable.
"""
import http.server
import importlib.util
import json
import os
from pathlib import Path
import selectors
import signal
import subprocess
import sys
import threading
import time

spec = importlib.util.spec_from_file_location("layout", Path(__file__).with_name("runtime_storage.py"))
layout = importlib.util.module_from_spec(spec)
spec.loader.exec_module(layout)
HOME = Path("/data/.codex/agents/recovery-test")
REPORT = Path("/results/storage-native-recovery.json")
TOKEN = "native-checkpoint-token-1184"


class Responses(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        payload = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        item = {"id": "msg_offline", "type": "message", "role": "assistant", "status": "completed", "content": [{"type": "output_text", "text": "Durable reply " + TOKEN, "annotations": []}]}
        response = {"id": "resp_offline", "object": "response", "status": "completed", "output": [item], "usage": {"input_tokens": 30, "output_tokens": 8, "total_tokens": 38, "input_tokens_details": {"cached_tokens": 0}}}
        if not payload.get("stream"):
            self.send_response(200); self.send_header("Content-Type", "application/json"); self.end_headers()
            self.wfile.write(json.dumps(response).encode()); return
        events = [
            {"type": "response.created", "response": {**response, "status": "in_progress", "output": []}},
            {"type": "response.output_item.added", "output_index": 0, "item": {**item, "status": "in_progress", "content": []}},
            {"type": "response.content_part.added", "output_index": 0, "item_id": item["id"], "content_index": 0, "part": {"type": "output_text", "text": "", "annotations": []}},
            {"type": "response.output_text.delta", "output_index": 0, "item_id": item["id"], "content_index": 0, "delta": item["content"][0]["text"]},
            {"type": "response.output_text.done", "output_index": 0, "item_id": item["id"], "content_index": 0, "text": item["content"][0]["text"]},
            {"type": "response.content_part.done", "output_index": 0, "item_id": item["id"], "content_index": 0, "part": item["content"][0]},
            {"type": "response.output_item.done", "output_index": 0, "item": item},
            {"type": "response.completed", "response": response},
        ]
        self.send_response(200); self.send_header("Content-Type", "text/event-stream"); self.end_headers()
        for event in events:
            self.wfile.write(("event: " + event["type"] + "\ndata: " + json.dumps(event) + "\n\n").encode())
        self.wfile.flush()


class RPC:
    def __init__(self, label):
        self.log = open("/results/recovery-" + label + ".stderr", "wb")
        self.p = subprocess.Popen(["/opt/memoh/toolkit/bin/codex", "app-server"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=self.log, cwd="/data", env={"CODEX_HOME": str(HOME), "PATH": "/opt/memoh/toolkit/bin:/usr/bin:/bin", "RUST_LOG": "error"}, start_new_session=True, bufsize=0)
        self.buffer = b""; self.counter = 0; self.notes = []
        self.call("initialize", {"clientInfo": {"name": "memoh-storage-recovery", "version": "1184"}})
        self.send({"method": "initialized"})

    def send(self, value):
        self.p.stdin.write((json.dumps(value) + "\n").encode())

    def receive(self, deadline):
        with selectors.DefaultSelector() as watcher:
            watcher.register(self.p.stdout, selectors.EVENT_READ)
            while time.monotonic() < deadline:
                if b"\n" in self.buffer:
                    line, self.buffer = self.buffer.split(b"\n", 1)
                    return json.loads(line)
                if not watcher.select(max(0, deadline - time.monotonic())):
                    break
                block = os.read(self.p.stdout.fileno(), 65536)
                if not block:
                    raise RuntimeError("app-server exited; inspect stderr")
                self.buffer += block
        raise TimeoutError("app-server RPC timeout")

    def call(self, method, params):
        self.counter += 1
        self.send({"id": self.counter, "method": method, "params": params})
        deadline = time.monotonic() + 20
        while True:
            value = self.receive(deadline)
            if value.get("id") == self.counter:
                if "error" in value:
                    raise RuntimeError(method + ": " + json.dumps(value["error"]))
                return value.get("result")
            self.notes.append(value)

    def turn(self, thread):
        self.call("turn/start", {"threadId": thread, "input": [{"type": "text", "text": TOKEN}]})
        deadline = time.monotonic() + 25
        while True:
            value = self.notes.pop(0) if self.notes else self.receive(deadline)
            if value.get("method") == "turn/completed":
                turn = value["params"]["turn"]
                if turn["status"] != "completed":
                    raise RuntimeError(json.dumps(turn))
                return turn

    def close(self):
        self.p.stdin.close()
        try:
            assert self.p.wait(timeout=10) == 0
        except subprocess.TimeoutExpired:
            os.killpg(self.p.pid, signal.SIGKILL); self.p.wait(timeout=5)
            raise
        finally:
            self.log.close()


def run(mode):
    if mode == "create":
        HOME.mkdir(parents=True, exist_ok=False)
    elif mode == "migrate":
        assert layout.migrate_drained(str(HOME)) == "migrated"
    else:
        assert layout.prepare(str(HOME)) == "isolated"
    report = json.loads(REPORT.read_text()) if REPORT.exists() else {}
    with http.server.ThreadingHTTPServer(("127.0.0.1", 0), Responses) as server:
        threading.Thread(target=server.serve_forever, daemon=True).start()
        (HOME / "config.toml").write_text('model = "gpt-5.4"\nmodel_provider = "offline"\n[model_providers.offline]\nname = "offline test"\nbase_url = "http://127.0.0.1:%d/v1"\nwire_api = "responses"\nrequires_openai_auth = false\n' % server.server_port)
        rpc = RPC(mode)
        try:
            if mode == "create":
                thread = rpc.call("thread/start", {"cwd": "/data", "approvalPolicy": "never"})["thread"]["id"]
                rpc.turn(thread)
                report["thread"] = thread
                report["installation_id"] = (HOME / "installation_id").read_text()
                report["created_native_turn"] = True
            else:
                thread = report["thread"]
                resumed = rpc.call("thread/resume", {"threadId": thread, "cwd": "/data", "approvalPolicy": "never"})
                assert resumed["thread"]["id"] == thread
                history = rpc.call("thread/read", {"threadId": thread, "includeTurns": True})
                assert TOKEN in json.dumps(history)
                assert (HOME / "installation_id").read_text() == report["installation_id"]
                rpc.turn(thread)
                fork = rpc.call("thread/fork", {"threadId": thread})["thread"]["id"]
                assert fork and fork != thread
                report[mode] = {"same_thread": True, "history_preserved": True, "same_installation_id": True, "new_turn_completed": True, "fork_succeeded": True, "home": str(HOME.resolve())}
        finally:
            rpc.close()
            server.shutdown()
    REPORT.write_text(json.dumps(report, indent=2))
    print(json.dumps(report))


if __name__ == "__main__":
    run(sys.argv[1])
