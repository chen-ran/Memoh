"""Scripted OpenAI-compatible provider for the MEMORY.md cache-cold QA run.

Chat turns get a short deterministic reply. Memory extraction turns every user
line into one fact and the memory decision adds every fact, so each turn writes
memory. Every chat request logs a hash of its "## MEMORY.md" system section and
a simulated exact-prefix cache hit (longest common prefix with any earlier chat
request in the last five minutes), the way OpenAI-style prefix caching matches.
"""

import hashlib
import json
import re
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LOG_PATH = sys.argv[1] if len(sys.argv) > 1 else "fake_llm.jsonl"
PORT = int(sys.argv[2]) if len(sys.argv) > 2 else 39190
CACHE_TTL = 300

lock = threading.Lock()
recent_prompts = []  # (timestamp, serialized prompt)


def text_of(content):
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(p.get("text", "") for p in content if isinstance(p, dict))
    return ""


def memory_section(system):
    start = system.find("## MEMORY.md")
    if start < 0:
        return None
    end = system.find("\n\n## PROFILES.md", start)
    return system[start:end] if end >= 0 else system[start:]


def log(entry):
    entry["ts"] = time.strftime("%Y-%m-%dT%H:%M:%S")
    with lock:
        with open(LOG_PATH, "a", encoding="utf-8") as f:
            f.write(json.dumps(entry, ensure_ascii=False) + "\n")


def common_prefix(a, b):
    n = min(len(a), len(b))
    i = 0
    while i < n and a[i] == b[i]:
        i += 1
    return i


def classify(system, messages):
    if "Personal Information Organizer" in system:
        return "extract"
    if "smart memory manager" in system:
        return "decide"
    if any(m.get("role") == "tool" for m in messages) or "## MEMORY.md" in system or len(system) > 4000:
        return "chat"
    return "other"


def extract_reply(user_text):
    facts = []
    for m in re.finditer(r"\[message_index=(\d+)\] User: (.*?)(?=\n\[message_index=\d+\]|\Z)", user_text, re.S):
        body = m.group(2).strip()
        body = re.sub(r"<[^>]+>", " ", body)
        body = " ".join(body.split())
        if body:
            facts.append({"text": "用户提到：" + body[-80:], "message_indices": [int(m.group(1))]})
    return json.dumps({"facts": facts}, ensure_ascii=False)


def decide_reply(user_text):
    marker = "The new retrieved facts are mentioned in the triple backticks"
    tail = user_text[user_text.find(marker):] if marker in user_text else user_text
    m = re.search(r"```\s*(\[.*?\])\s*```", tail, re.S)
    facts = json.loads(m.group(1)) if m else []
    actions = [
        {"id": str(i), "text": f.get("text", ""), "event": "ADD", "source_fact_indices": [f.get("index", i)]}
        for i, f in enumerate(facts)
    ]
    return json.dumps({"memory": actions}, ensure_ascii=False)


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def send_json(self, status, body):
        data = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path.rstrip("/").endswith("/models"):
            self.send_json(200, {"object": "list", "data": [{"id": "qa-cache-model", "object": "model", "owned_by": "qa"}]})
            return
        self.send_json(404, {"error": {"message": "not found"}})

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        req = json.loads(self.rfile.read(length) or b"{}")
        if not self.path.rstrip("/").endswith("/chat/completions"):
            self.send_json(404, {"error": {"message": "not found"}})
            return
        messages = req.get("messages", [])
        system = "\n".join(text_of(m.get("content")) for m in messages if m.get("role") in ("system", "developer"))
        users = [text_of(m.get("content")) for m in messages if m.get("role") == "user"]
        last_user = users[-1] if users else ""
        kind = classify(system, messages)

        serialized = json.dumps(messages, ensure_ascii=False, sort_keys=True)
        now = time.time()
        with lock:
            recent_prompts[:] = [(t, p) for t, p in recent_prompts if now - t < CACHE_TTL]
            hit = max((common_prefix(serialized, p) for _, p in recent_prompts), default=0) if kind == "chat" else 0
            if kind == "chat":
                recent_prompts.append((now, serialized))

        if kind == "extract":
            reply = extract_reply(last_user)
        elif kind == "decide":
            reply = decide_reply(last_user)
        elif kind == "chat":
            visible = " ".join(re.sub(r"<[^>]+>", " ", last_user).split())
            reply = "收到：" + visible[-60:]
        else:
            reply = "MEMORY.md 缓存验证"

        prompt_tokens = max(1, len(serialized) // 4)
        cached_tokens = (hit // 4) // 64 * 64
        section = memory_section(system)
        entry = {
            "kind": kind,
            "model": req.get("model"),
            "stream": bool(req.get("stream")),
            "messages": len(messages),
            "system_sha": hashlib.sha256(system.encode()).hexdigest()[:12],
        }
        if kind == "chat":
            entry.update({
                "memory_md_sha": hashlib.sha256(section.encode()).hexdigest()[:12] if section is not None else None,
                "memory_md_entries": len(re.findall(r"^\d+\. \[", section or "", re.M)),
                "memory_md_offset": system.find("## MEMORY.md"),
                "prompt_chars": len(serialized),
                "simulated_prefix_hit_chars": hit,
                "simulated_hit_ratio": round(hit / len(serialized), 3) if serialized else 0,
                "last_user": last_user[-80:],
            })
        log(entry)

        usage = {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": max(1, len(reply) // 4),
            "total_tokens": prompt_tokens + max(1, len(reply) // 4),
            "prompt_tokens_details": {"cached_tokens": min(cached_tokens, prompt_tokens)},
        }
        rid = "chatcmpl-qa-%d" % int(now * 1000)
        if not req.get("stream"):
            self.send_json(200, {
                "id": rid, "object": "chat.completion", "created": int(now), "model": req.get("model"),
                "choices": [{"index": 0, "message": {"role": "assistant", "content": reply}, "finish_reason": "stop"}],
                "usage": usage,
            })
            return

        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()

        def chunk(delta, finish=None, extra=None):
            body = {"id": rid, "object": "chat.completion.chunk", "created": int(now), "model": req.get("model"),
                    "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
            if extra:
                body.update(extra)
            self.wfile.write(("data: " + json.dumps(body, ensure_ascii=False) + "\n\n").encode())
            self.wfile.flush()

        chunk({"role": "assistant", "content": ""})
        for i in range(0, len(reply), 8):
            chunk({"content": reply[i:i + 8]})
        chunk({}, "stop")
        usage_chunk = {"id": rid, "object": "chat.completion.chunk", "created": int(now), "model": req.get("model"),
                       "choices": [], "usage": usage}
        self.wfile.write(("data: " + json.dumps(usage_chunk) + "\n\n").encode())
        self.wfile.write(b"data: [DONE]\n\n")
        self.wfile.flush()
        self.close_connection = True


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
