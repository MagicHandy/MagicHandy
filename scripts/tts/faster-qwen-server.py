#!/usr/bin/env python3
"""MagicHandy launcher for the pinned faster-qwen3-tts OpenAI server.

The upstream server intentionally follows the standard OpenAI request shape and
does not expose generation seeds or Base-model instructions. MagicHandy adds an
optional unsigned seed and tone instruction while retaining upstream's single
inference lock.
"""

import asyncio
import importlib.util
import math
import random
import sys
from contextlib import aclosing
from pathlib import Path
from typing import AsyncGenerator, Optional

import torch
from fastapi import FastAPI, HTTPException
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field
from tts_stream import PrivateWorkerAPI, stream_from_sync


def load_upstream(path: str):
    source = Path(path).resolve()
    if not source.is_file():
        raise RuntimeError(f"upstream Faster Qwen server is unavailable: {source}")
    spec = importlib.util.spec_from_file_location("magichandy_faster_qwen_upstream", source)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"could not load upstream Faster Qwen server: {source}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


if len(sys.argv) < 2:
    raise SystemExit("usage: faster-qwen-server.py UPSTREAM_SERVER [SERVER_ARGS...]")

upstream = load_upstream(sys.argv[1])
sys.argv = [sys.argv[1], *sys.argv[2:]]

app = FastAPI(title="MagicHandy Faster Qwen3-TTS API", docs_url=None, redoc_url=None, openapi_url=None)
app.add_middleware(PrivateWorkerAPI)
DEFAULT_SEED = 1337
MIN_GENERATION_SECONDS = 12
MAX_GENERATION_SECONDS = 160


class SpeechRequest(BaseModel):
    model: str = "tts-1"
    input: str = Field(max_length=32768)
    voice: str = "alloy"
    response_format: str = "wav"
    speed: float = 1.0
    seed: int = Field(default=DEFAULT_SEED, ge=0, le=0xFFFFFFFF)
    instruct: str = Field(default="", max_length=2048)


def seed_generators(seed: int) -> None:
    random.seed(seed)
    upstream.np.random.seed(seed)
    torch.manual_seed(seed)
    if torch.cuda.is_available():
        torch.cuda.manual_seed_all(seed)


def max_generation_tokens(text: str) -> int:
    """Give short prompts room to finish without allowing multi-minute loops."""
    words = len(text.split())
    estimated_seconds = 8 + max(words * 1.5, len(text.strip()) / 4)
    bounded_seconds = max(MIN_GENERATION_SECONDS, min(MAX_GENERATION_SECONDS, estimated_seconds))
    return math.ceil(bounded_seconds * 12)


def warm_up_model() -> None:
    """Warm the complete streaming path with one discarded codec frame."""
    voice_cfg = upstream.resolve_voice(upstream.default_voice)
    with upstream._model_lock:
        seed_generators(DEFAULT_SEED)
        stream = upstream.tts_model.generate_voice_clone_streaming(
            text="Ready.",
            language=voice_cfg.get("language", "Auto"),
            ref_audio=voice_cfg["ref_audio"],
            ref_text=voice_cfg.get("ref_text", ""),
            max_new_tokens=2,
            min_new_tokens=1,
            chunk_size=1,
            non_streaming_mode=False,
        )
        try:
            next(stream)
        except StopIteration:
            pass
        finally:
            stream.close()
    upstream.logger.info("MagicHandy one-frame streaming warm-up complete")


async def stream_chunks(
    voice_cfg: dict, text: str, seed: int, instruct: str
) -> AsyncGenerator[bytes, None]:
    def generate(canceled):
        with upstream._model_lock:
            if canceled.is_set():
                return
            seed_generators(seed)
            stream = upstream.tts_model.generate_voice_clone_streaming(
                text=text,
                language=voice_cfg.get("language", "Auto"),
                ref_audio=voice_cfg["ref_audio"],
                ref_text=voice_cfg.get("ref_text", ""),
                max_new_tokens=max_generation_tokens(text),
                chunk_size=voice_cfg.get("chunk_size", 12),
                non_streaming_mode=False,
                instruct=instruct or None,
            )
            try:
                for chunk, sample_rate, _timing in stream:
                    if canceled.is_set():
                        break
                    if sample_rate != upstream.SAMPLE_RATE:
                        raise RuntimeError("TTS sample rate changed during inference")
                    yield upstream._to_pcm16(chunk)
            finally:
                stream.close()

    async with aclosing(stream_from_sync(generate)) as chunks:
        async for chunk in chunks:
            yield chunk


@app.get("/health")
async def health():
    return {"status": "ok", "model_loaded": upstream.tts_model is not None}


@app.post("/v1/audio/speech")
async def create_speech(request: SpeechRequest):
    if upstream.tts_model is None:
        raise HTTPException(status_code=503, detail="Model not loaded")
    if not request.input.strip():
        raise HTTPException(status_code=400, detail="'input' text is empty")

    voice_cfg = upstream.resolve_voice(request.voice)
    output_format = request.response_format.lower()
    content_types = {
        "wav": "audio/wav",
        "pcm": "audio/pcm",
        "mp3": "audio/mpeg",
    }
    if output_format not in content_types:
        raise HTTPException(
            status_code=400,
            detail=f"response_format {output_format!r} not supported. Use: wav, pcm, mp3",
        )

    if output_format == "mp3":
        async def mp3_stream():
            # Use the same cancelable producer as WAV/PCM. A disconnect skips
            # remaining model chunks even when the chosen codec needs a full clip.
            pcm = bytearray()
            async with aclosing(stream_chunks(voice_cfg, request.input, request.seed, request.instruct)) as chunks:
                async for chunk in chunks:
                    if len(pcm) + len(chunk) > 8 * 1024 * 1024:
                        raise HTTPException(status_code=413, detail="TTS audio exceeds 8 MiB")
                    pcm.extend(chunk)
            audio = upstream.np.frombuffer(pcm, dtype="<i2").astype(upstream.np.float32) / 32768.0
            encoded = await asyncio.to_thread(upstream._to_mp3_bytes, audio, upstream.SAMPLE_RATE)
            yield encoded
        return StreamingResponse(mp3_stream(), media_type=content_types[output_format])

    async def audio_stream():
        if output_format == "wav":
            yield upstream._wav_header(upstream.SAMPLE_RATE)
        async with aclosing(stream_chunks(voice_cfg, request.input, request.seed, request.instruct)) as chunks:
            async for raw_chunk in chunks:
                yield raw_chunk

    return StreamingResponse(audio_stream(), media_type=content_types[output_format])


if __name__ == "__main__":
    run_server = upstream.uvicorn.run

    def run_after_warmup(*args, **kwargs):
        warm_up_model()
        return run_server(*args, **kwargs)

    upstream.app = app
    upstream.uvicorn.run = run_after_warmup
    upstream.main()
