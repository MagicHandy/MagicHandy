"""Private MagicHandy adapter for the pinned MIT-licensed Chatterbox engine.

Keep upstream voice resolution, seeds, sentence splitting and crossfades while
bounding buffering and moving inference off the ASGI loop. The upstream
management UI and configuration/upload endpoints are not mounted.
"""

import asyncio
import importlib.util
import struct
import sys
from contextlib import aclosing, asynccontextmanager
from pathlib import Path

from fastapi import FastAPI, HTTPException
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field

from tts_stream import PrivateWorkerAPI, stream_from_sync

SAMPLE_RATE = 24000
MAX_AUDIO_BYTES = 8 * 1024 * 1024


class SpeechRequest(BaseModel):
    model: str = "tts-1"
    input_: str = Field(alias="input", min_length=1, max_length=32768)
    voice: str
    response_format: str = "wav"
    speed: float = Field(default=1.0, ge=0.25, le=4.0)
    seed: int | None = Field(default=None, ge=0, le=0xFFFFFFFF)
    language: str | None = None


def wav_header():
    return struct.pack("<4sI4s4sIHHIIHH4sI", b"RIFF", 0xFFFFFFFF, b"WAVE", b"fmt ",
                       16, 1, 1, SAMPLE_RATE, SAMPLE_RATE * 2, 2, 16, b"data", 0xFFFFFFFF)


def resolve_voice(upstream, voice):
    for base in (upstream.get_predefined_voices_path(ensure_absolute=True),
                 upstream.get_reference_audio_path(ensure_absolute=True)):
        try:
            path = upstream.utils.safe_resolve_within(base, voice)
        except ValueError as error:
            raise HTTPException(status_code=400, detail="Invalid voice parameter") from error
        if path.is_file():
            return path
    raise HTTPException(status_code=404, detail="Voice file was not found")


def synthesize_chunks(upstream, request, voice_path, texts, canceled):
    """Keep only one sentence plus a small crossfade tail in memory."""
    np = upstream.np
    seed = request.seed if request.seed is not None else upstream.get_gen_default_seed()
    carry = None
    total = 0
    fade = SAMPLE_RATE // 50  # Upstream: 20 ms fades, 200 ms sentence pause.
    for index, text in enumerate(texts):
        if canceled.is_set():
            return
        tensor, sample_rate = upstream.engine.synthesize(
            text=text,
            audio_prompt_path=str(voice_path),
            temperature=upstream.get_gen_default_temperature(),
            exaggeration=upstream.get_gen_default_exaggeration(),
            cfg_weight=upstream.get_gen_default_cfg_weight(),
            seed=(seed + index) % (2**32) if seed is not None and seed >= 0 else seed,
            language=request.language or upstream.get_gen_default_language(),
        )
        if canceled.is_set():
            return
        if tensor is None or sample_rate != SAMPLE_RATE:
            raise RuntimeError("Chatterbox did not return 24 kHz audio")
        if request.speed != 1.0:
            tensor, _ = upstream.utils.apply_speed_factor(tensor, sample_rate, request.speed)
        samples = tensor.cpu().numpy().reshape(-1).astype(np.float32)
        if not samples.size or not np.isfinite(samples).all():
            raise RuntimeError("Chatterbox returned empty or invalid audio")
        # Streaming cannot normalize against the peak of a future sentence.
        # Limit each sentence before crossfading to prevent PCM clipping.
        peak = float(np.abs(samples).max())
        if peak > 0.99:
            samples *= 0.95 / peak
        if carry is not None:
            samples = upstream._crossfade_with_overlap(carry, samples, fade)
        if index + 1 < len(texts):
            silence = np.zeros(SAMPLE_RATE // 5 + fade * 2, dtype=np.float32)
            samples = upstream._crossfade_with_overlap(samples, silence, fade)
            carry = samples[-fade:].copy()
            samples = samples[:-fade]
        pcm = (np.clip(samples, -1, 1) * 32767).astype("<i2").tobytes()
        total += len(pcm)
        if total > MAX_AUDIO_BYTES:
            raise RuntimeError("TTS audio exceeds 8 MiB")
        for offset in range(0, len(pcm), 32768):
            if canceled.is_set():
                return
            yield pcm[offset:offset + 32768]


def create_app(upstream):
    @asynccontextmanager
    async def lifespan(_app):
        if not await asyncio.to_thread(upstream.engine.load_model):
            raise RuntimeError("Chatterbox model failed to load")
        try:
            yield
        finally:
            await asyncio.to_thread(upstream.engine.unload_model)

    app = FastAPI(title="MagicHandy Chatterbox API", lifespan=lifespan,
                  docs_url=None, redoc_url=None, openapi_url=None)
    app.add_middleware(PrivateWorkerAPI)

    @app.get("/health")
    @app.get("/api/model-info")
    async def health():
        return {"status": "ok", "loaded": bool(upstream.engine.MODEL_LOADED),
                "model_loaded": bool(upstream.engine.MODEL_LOADED)}

    @app.post("/v1/audio/speech")
    async def speech(request: SpeechRequest):
        if not upstream.engine.MODEL_LOADED:
            raise HTTPException(status_code=503, detail="Model not loaded")
        output_format = request.response_format.lower()
        if output_format not in {"wav", "pcm", "mp3", "opus"}:
            raise HTTPException(status_code=400, detail="Unsupported audio format")
        voice_path = resolve_voice(upstream, request.voice)
        texts = upstream.utils.chunk_text_by_sentences(request.input_, 120)
        if not texts:
            raise HTTPException(status_code=400, detail="Speech text is empty")

        async def audio():
            if output_format == "wav":
                yield wav_header()
            buffered = bytearray()
            generate = lambda canceled: synthesize_chunks(upstream, request, voice_path, texts, canceled)
            async with aclosing(stream_from_sync(generate)) as chunks:
                async for chunk in chunks:
                    if output_format in {"wav", "pcm"}:
                        yield chunk
                    else:
                        buffered.extend(chunk)
            if output_format not in {"wav", "pcm"}:
                samples = upstream.np.frombuffer(buffered, dtype="<i2").astype(upstream.np.float32) / 32768.0
                encoded = await asyncio.to_thread(upstream.utils.encode_audio, audio_array=samples,
                    sample_rate=SAMPLE_RATE, output_format=output_format, target_sample_rate=SAMPLE_RATE)
                if not encoded:
                    raise RuntimeError("Chatterbox audio encoding failed")
                yield encoded

        content_type = {"wav": "audio/wav", "pcm": "audio/pcm", "mp3": "audio/mpeg", "opus": "audio/ogg"}[output_format]
        return StreamingResponse(audio(), media_type=content_type)

    return app


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: chatterbox-server.py <upstream-server.py>")
    server = Path(sys.argv[1]).resolve()
    if not server.is_file():
        raise SystemExit(f"Chatterbox server is unavailable: {server}")
    sys.path.insert(0, str(server.parent))
    spec = importlib.util.spec_from_file_location("magichandy_chatterbox_upstream", server)
    upstream = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = upstream
    spec.loader.exec_module(upstream)
    import uvicorn
    uvicorn.run(create_app(upstream), host="127.0.0.1", port=upstream.get_port(),
                log_level="info", access_log=False, workers=1)


if __name__ == "__main__":
    main()
