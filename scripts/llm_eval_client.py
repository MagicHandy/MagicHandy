"""Small standard-library client for isolated full-build LLM evaluations.

Use only an app started with -simulate-motion and a disposable data directory.
Reports belong outside git: they can contain model output and session text.
"""

import json, threading, time, urllib.request, urllib.error, urllib.parse


class App:
    def __init__(self, base):
        if urllib.parse.urlparse(base).hostname not in [
            "127.0.0.1",
            "localhost",
            "::1",
        ]:
            raise ValueError("Evaluation requires a loopback simulator app.")
        self.base = base.rstrip("/")
        self.headers = {
            "Content-Type": "application/json",
            "X-MagicHandy-Client-ID": "llama-autopilot-evaluation",
        }
        self.done = threading.Event()

    def request(self, method, path, body=None, raw=False, timeout=150):
        req = urllib.request.Request(
            self.base + path,
            data=None if body is None else json.dumps(body).encode(),
            method=method,
            headers=self.headers,
        )
        for attempt in range(3):
            try:
                with urllib.request.urlopen(req, timeout=timeout) as r:
                    text = r.read().decode()
                return text if raw else json.loads(text)
            except urllib.error.HTTPError as e:
                raise RuntimeError(
                    f"{method} {path}: {e.code} {e.read().decode()}"
                ) from e
            except OSError:
                # Retry only reads and unconditional Stop after a transient
                # loopback reset. Never replay chat or a motion-start mutation.
                if attempt == 2 or (method != "GET" and path != "/api/motion/stop"):
                    raise
                time.sleep(0.1 * (attempt + 1))

    def claim(self):
        if not self.request("GET", "/api/state").get("motion_simulated"):
            raise RuntimeError(
                "Evaluation requires -simulate-motion before acquiring control."
            )
        state = self.request("POST", "/api/controller/takeover", {})
        self.headers["X-MagicHandy-Stop-Sequence"] = str(state["stop_sequence"])

        def renew():
            while not self.done.wait(1):
                try:
                    c = self.request("GET", "/api/controller", timeout=3)
                    self.headers["X-MagicHandy-Stop-Sequence"] = str(
                        c.get(
                            "stop_sequence", self.headers["X-MagicHandy-Stop-Sequence"]
                        )
                    )
                except Exception:
                    pass

        self.worker = threading.Thread(target=renew, daemon=True)
        self.worker.start()

    def check_model(self):
        state = self.request("GET", "/api/llm/status")
        if not all(state.get(k) for k in ["available", "model_available", "loaded"]):
            raise RuntimeError(
                "The exact configured model must be loaded before evaluation: "
                + str(state.get("message", "unavailable"))
            )

    def close(self):
        self.done.set()
        self.worker.join()

    def settings(self):
        return self.request("GET", "/api/settings")["settings"]

    def save(self, settings):
        payload = {
            k: settings[k]
            for k in [
                "server",
                "ui",
                "labs",
                "device",
                "motion",
                "autopilot",
                "llm",
                "voice",
                "chat",
                "diagnostics",
            ]
        }
        payload["device"] = {
            k: v
            for k, v in payload["device"].items()
            if k
            in [
                "hsp_dispatch_owner",
                "intiface_server_address",
                "firmware_api_requirement",
                "api_application_id_source",
                "api_application_id_override",
            ]
        }
        payload["voice"] = {
            k: v for k, v in payload["voice"].items() if not k.endswith("_key_set")
        }
        return self.request("PUT", "/api/settings", payload)

    def stop(self):
        result = self.request("POST", "/api/motion/stop", {})
        c = self.request("POST", "/api/controller/takeover", {})
        self.headers["X-MagicHandy-Stop-Sequence"] = str(
            c.get("stop_sequence", self.headers["X-MagicHandy-Stop-Sequence"])
        )
        return result
