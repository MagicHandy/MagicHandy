"""Optional loopback recorder for isolated llama.cpp evaluation builds.

Point a comparison app's external llama.cpp URL at this recorder. The upstream
worker must remain owned by a separate app or process. Captures contain prompts
and replies: use disposable sessions and keep the output outside git.
"""

import argparse
import json
import threading
import time
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
source = parser.add_mutually_exclusive_group(required=True)
source.add_argument("--upstream")
source.add_argument(
    "--upstream-file",
    type=Path,
    help="JSON containing base_url; reread after managed port changes",
)
parser.add_argument("--port", type=int, default=8479)
parser.add_argument("--output", type=Path, required=True)
args = parser.parse_args()
lock = threading.Lock()


def upstream():
    value = args.upstream
    if args.upstream_file:
        value = json.loads(args.upstream_file.read_text(encoding="utf-8-sig"))[
            "base_url"
        ]
    parsed = urllib.parse.urlparse(value)
    if (
        parsed.scheme not in ["http", "https"]
        or parsed.hostname not in ["127.0.0.1", "localhost", "::1"]
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError(
            "The recorder accepts only a credential-free loopback upstream."
        )
    return value.rstrip("/")


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        self.forward()

    def do_POST(self):
        self.forward()

    def forward(self):
        if self.path not in ["/health", "/v1/models", "/v1/chat/completions"]:
            self.send_error(404)
            return
        size = int(self.headers.get("Content-Length", "0"))
        if size < 0 or size > 2 * 1024 * 1024:
            self.send_error(413)
            return
        body = self.rfile.read(size)
        started = time.monotonic()
        captured = bytearray()
        sent = False
        error = ""
        try:
            request = urllib.request.Request(
                upstream() + self.path,
                data=body or None,
                method=self.command,
                headers={"Content-Type": "application/json"},
            )
            with urllib.request.urlopen(request, timeout=180) as response:
                self.send_response(response.status)
                self.send_header(
                    "Content-Type",
                    response.headers.get("Content-Type", "application/json"),
                )
                self.end_headers()
                sent = True
                for line in response:
                    if len(captured) < 8 * 1024 * 1024:
                        captured.extend(line[: 8 * 1024 * 1024 - len(captured)])
                    self.wfile.write(line)
                    self.wfile.flush()
        except Exception as exc:
            error = str(exc)
            if not sent:
                self.send_error(502, "Local model unavailable")
        finally:
            if self.path == "/v1/chat/completions":
                record = {
                    "at": time.time(),
                    "elapsed_ms": round((time.monotonic() - started) * 1000),
                    "request": body.decode(errors="replace"),
                    "response": captured.decode(errors="replace"),
                    "capture_limit_reached": len(captured) == 8 * 1024 * 1024,
                    "error": error,
                }
                with lock:
                    with args.output.open("a", encoding="utf-8") as file:
                        file.write(json.dumps(record) + "\n")


upstream()
ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()
