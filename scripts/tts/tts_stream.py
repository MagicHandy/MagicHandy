"""Bounded, cancellation-aware bridge from synchronous inference to ASGI.

No executor thread blocks in Queue.get. Completion uses a separate event so a
full audio queue cannot strand the producer while delivering a sentinel.
"""

import asyncio
import queue
import threading
from urllib.parse import urlsplit

_inference = threading.Lock()
_producers = threading.BoundedSemaphore(2)


async def stream_from_sync(generate):
    """Yield byte chunks from generate(canceled), with at most two queued chunks.

    A running GPU call must return before it can stop. Canceled waiters never
    enter inference, and no new call overlaps the previous model invocation.
    """
    if not _producers.acquire(blocking=False):
        raise RuntimeError("TTS inference queue is full")
    chunks = queue.Queue(maxsize=2)
    canceled = threading.Event()
    finished = threading.Event()
    wake = asyncio.Event()
    loop = asyncio.get_running_loop()

    def notify():
        try:
            loop.call_soon_threadsafe(wake.set)
        except RuntimeError:
            canceled.set()  # The ASGI loop has already shut down.

    def put(item):
        while not canceled.is_set():
            try:
                chunks.put(item, timeout=0.05)
                notify()
                return True
            except queue.Full:
                pass
        return False

    def produce():
        acquired = False
        source = None
        try:
            while not canceled.is_set():
                if _inference.acquire(timeout=0.05):
                    acquired = True
                    break
            if canceled.is_set():
                return
            source = generate(canceled)
            while not canceled.is_set():
                try:
                    item = next(source)
                except StopIteration:
                    break
                if not put(item):
                    break
        except Exception as error:
            put(error)
        finally:
            try:
                if source is not None:
                    source.close()
            finally:
                if acquired:
                    _inference.release()
                finished.set()
                _producers.release()
                notify()

    thread = threading.Thread(target=produce, name="magichandy-tts-inference", daemon=True)
    try:
        thread.start()
    except BaseException:
        _producers.release()
        raise
    try:
        while True:
            wake.clear()
            try:
                item = chunks.get_nowait()
            except queue.Empty:
                if finished.is_set():
                    # The producer sets finished only after its last put. A
                    # final put can race the empty observation above.
                    if chunks.empty():
                        break
                    continue
                await wake.wait()
                continue
            if isinstance(item, Exception):
                raise item
            yield item
    finally:
        canceled.set()


class PrivateWorkerAPI:
    """A child API is for the local Go worker, never a browser control panel."""

    def __init__(self, app):
        self.app = app

    async def __call__(self, scope, receive, send):
        if scope["type"] == "http":
            headers = {key.lower(): value for key, value in scope.get("headers", [])}
            host = headers.get(b"host", b"").decode("ascii", "replace")
            try:
                hostname = urlsplit("//" + host).hostname
            except ValueError:
                hostname = None
            if b"origin" in headers or hostname not in {"127.0.0.1", "localhost", "::1"}:
                await send({"type": "http.response.start", "status": 403, "headers": [(b"content-type", b"text/plain")]})
                await send({"type": "http.response.body", "body": b"Private worker API"})
                return
        await self.app(scope, receive, send)
