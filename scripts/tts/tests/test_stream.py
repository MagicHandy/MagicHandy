import asyncio
import importlib.util
import sys
import threading
import time
import unittest
import tempfile
from pathlib import Path
from types import SimpleNamespace, ModuleType
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from tts_stream import PrivateWorkerAPI, stream_from_sync


async def wait_event(event):
    for _ in range(100):
        if event.is_set():
            return
        await asyncio.sleep(0.01)
    raise AssertionError("inference producer did not settle")


class StreamingTests(unittest.IsolatedAsyncioTestCase):
    async def test_full_queue_completion_and_disconnect_do_not_strand_producer(self):
        for count in (3, 100):
            closed = threading.Event()

            def generate(canceled):
                try:
                    for _ in range(count):
                        yield b"audio"
                finally:
                    closed.set()

            stream = stream_from_sync(generate)
            self.assertEqual(await anext(stream), b"audio")
            await asyncio.sleep(0.02)  # Fill both queue positions.
            await stream.aclose()
            await wait_event(closed)

    async def test_inference_does_not_block_event_loop_and_cancel_skips_next_call(self):
        entered, release, closed = threading.Event(), threading.Event(), threading.Event()
        calls = []

        def generate(canceled):
            try:
                entered.set()
                release.wait(2)
                for index in range(10):
                    if canceled.is_set():
                        return
                    calls.append(index)
                    yield b"audio"
            finally:
                closed.set()

        stream = stream_from_sync(generate)
        pending = asyncio.create_task(anext(stream))
        try:
            await wait_event(entered)
            heartbeat = time.monotonic()
            await asyncio.sleep(0.02)
            self.assertLess(time.monotonic() - heartbeat, 0.5)
            pending.cancel()
            with self.assertRaises(asyncio.CancelledError):
                await pending
        finally:
            release.set()
            await stream.aclose()
            await wait_event(closed)
        self.assertEqual(calls, [])

    async def test_normal_order_and_errors(self):
        def success(canceled):
            yield b"one"
            yield b"two"

        self.assertEqual([chunk async for chunk in stream_from_sync(success)], [b"one", b"two"])

        def failure(canceled):
            yield b"one"
            raise ValueError("inference failed")

        with self.assertRaisesRegex(ValueError, "inference failed"):
            _ = [chunk async for chunk in stream_from_sync(failure)]


class PrivateAPITests(unittest.IsolatedAsyncioTestCase):
    async def test_rejects_browser_origins_and_rebinding_hosts(self):
        calls = []

        async def app(scope, receive, send):
            calls.append(scope)

        guard = PrivateWorkerAPI(app)
        for headers in ([(b"host", b"127.0.0.1:8992"), (b"origin", b"null")],
                        [(b"host", b"malicious.example:8992")]):
            responses = []

            async def send(response):
                responses.append(response)

            await guard({"type": "http", "headers": headers}, None, send)
            self.assertEqual(responses[0]["status"], 403)
        self.assertEqual(calls, [])
        await guard({"type": "http", "headers": [(b"host", b"127.0.0.1:8992")]}, None, None)
        self.assertEqual(len(calls), 1)


def load_chatterbox():
    path = Path(__file__).resolve().parents[1] / "chatterbox-server.py"
    spec = importlib.util.spec_from_file_location("chatterbox_adapter_test", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ChatterboxTests(unittest.IsolatedAsyncioTestCase):
    async def test_private_app_has_no_management_routes(self):
        import httpx
        module = load_chatterbox()
        app = module.create_app(SimpleNamespace(engine=SimpleNamespace(MODEL_LOADED=True)))
        async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://127.0.0.1") as client:
            self.assertEqual((await client.get("/health")).json()["loaded"], True)
            for path in ("/docs", "/api/config", "/save_settings", "/upload_audio", "/api/model/unload"):
                self.assertEqual((await client.post(path)).status_code, 404)
            self.assertEqual((await client.get("/health", headers={"Origin": "https://example.com"})).status_code, 403)

    async def test_sentence_stream_cancels_before_next_inference_and_keeps_seed(self):
        import numpy as np
        module = load_chatterbox()
        calls = []
        canceled = threading.Event()

        class Tensor:
            def cpu(self): return self
            def numpy(self): return np.ones(2400, dtype=np.float32) * 0.1

        def synthesize(**kwargs):
            calls.append(kwargs)
            return Tensor(), 24000

        upstream = SimpleNamespace(np=np, engine=SimpleNamespace(synthesize=synthesize),
            get_gen_default_seed=lambda: 1337, get_gen_default_temperature=lambda: 0.8,
            get_gen_default_exaggeration=lambda: 0.5, get_gen_default_cfg_weight=lambda: 0.5,
            get_gen_default_language=lambda: "en",
            _crossfade_with_overlap=lambda a, b, fade: np.concatenate((a[:-fade], a[-fade:] + b[:fade], b[fade:])))
        request = module.SpeechRequest(input="hello", voice="voice.wav", seed=7)
        stream = module.synthesize_chunks(upstream, request, Path("voice.wav"), ["one", "two"], canceled)
        self.assertGreater(len(next(stream)), 0)
        self.assertEqual(calls[0]["seed"], 7)
        canceled.set()
        self.assertEqual(list(stream), [])
        self.assertEqual(len(calls), 1)


class QwenTests(unittest.IsolatedAsyncioTestCase):
    async def test_real_adapter_forwards_conditioning_and_closes_canceled_generator(self):
        import numpy as np
        torch = ModuleType("torch")
        torch.manual_seed = lambda seed: None
        torch.cuda = SimpleNamespace(is_available=lambda: False)
        launcher = Path(__file__).resolve().parents[1] / "faster-qwen-server.py"
        with tempfile.TemporaryDirectory() as directory:
            upstream_file = Path(directory) / "upstream.py"
            upstream_file.write_text("SAMPLE_RATE = 24000\n", encoding="utf-8")
            with patch.dict(sys.modules, {"torch": torch}), patch.object(sys, "argv", [str(launcher), str(upstream_file)]):
                spec = importlib.util.spec_from_file_location("qwen_adapter_test", launcher)
                module = importlib.util.module_from_spec(spec)
                spec.loader.exec_module(module)
            closed = threading.Event()
            received = {}

            def generate(**kwargs):
                received.update(kwargs)
                try:
                    for _ in range(100):
                        yield b"\x00\x01", 24000, {}
                finally:
                    closed.set()

            module.upstream.np = np
            module.upstream._model_lock = threading.Lock()
            module.upstream._to_pcm16 = lambda chunk: chunk
            module.upstream.tts_model = SimpleNamespace(generate_voice_clone_streaming=generate)
            stream = module.stream_chunks({"ref_audio": "fixture.wav", "ref_text": "Reference.", "language": "English"}, "Hello.", 11, "Quietly.")
            self.assertEqual(await anext(stream), b"\x00\x01")
            await stream.aclose()
            await wait_event(closed)
            self.assertEqual(received["instruct"], "Quietly.")
            self.assertEqual(received["ref_text"], "Reference.")
            self.assertFalse(received["non_streaming_mode"])
            self.assertLessEqual(received["max_new_tokens"], 160 * 12)


if __name__ == "__main__":
    unittest.main()
