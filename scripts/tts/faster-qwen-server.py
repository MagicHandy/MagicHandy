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
import queue
import random
import sys
import threading
from pathlib import Path
from typing import AsyncGenerator, Optional

import torch
from fastapi import FastAPI, HTTPException
from fastapi.responses import Response, StreamingResponse
from pydantic import BaseModel, Field


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

app = FastAPI(title="MagicHandy Faster Qwen3-TTS API")
DEFAULT_SEED = 1337
MIN_GENERATION_SECONDS = 12
MAX_GENERATION_SECONDS = 160


class SpeechRequest(BaseModel):
    model: str = "tts-1"
    input: str
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
    """Consume one short hidden generation before the server reports ready."""
    voice_cfg = upstream.resolve_voice(upstream.default_voice)
    with upstream._model_lock:
        seed_generators(DEFAULT_SEED)
        stream = upstream.tts_model.generate_voice_clone_streaming(
            text="Ready.",
            language=voice_cfg.get("language", "Auto"),
            ref_audio=voice_cfg["ref_audio"],
            ref_text=voice_cfg.get("ref_text", ""),
            max_new_tokens=max_generation_tokens("Ready."),
            chunk_size=voice_cfg.get("chunk_size", 12),
            non_streaming_mode=False,
        )
        try:
            next(stream)
        except StopIteration:
            pass
        finally:
            stream.close()
    upstream.logger.info("MagicHandy streaming warm-up complete")


async def stream_chunks(
    voice_cfg: dict, text: str, seed: int, instruct: str
) -> AsyncGenerator[bytes, None]:
    chunks: queue.Queue = queue.Queue()
    done = object()

    def producer() -> None:
        try:
            with upstream._model_lock:
                seed_generators(seed)
                token_limit = max_generation_tokens(text)
                upstream.logger.info(
                    "MagicHandy generation seed=%d max_new_tokens=%d", seed, token_limit
                )
                for chunk, _sample_rate, _timing in upstream.tts_model.generate_voice_clone_streaming(
                    text=text,
                    language=voice_cfg.get("language", "Auto"),
                    ref_audio=voice_cfg["ref_audio"],
                    ref_text=voice_cfg.get("ref_text", ""),
                    max_new_tokens=token_limit,
                    chunk_size=voice_cfg.get("chunk_size", 12),
                    non_streaming_mode=False,
                    instruct=instruct or None,
                ):
                    chunks.put(chunk)
        except Exception as exc:
            chunks.put(exc)
        finally:
            chunks.put(done)

    threading.Thread(target=producer, daemon=True).start()
    loop = asyncio.get_running_loop()
    while True:
        item = await loop.run_in_executor(None, chunks.get)
        if item is done:
            break
        if isinstance(item, Exception):
            raise item
        yield upstream._to_pcm16(item)


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
        loop = asyncio.get_running_loop()

        def generate():
            with upstream._model_lock:
                seed_generators(request.seed)
                token_limit = max_generation_tokens(request.input)
                upstream.logger.info(
                    "MagicHandy generation seed=%d max_new_tokens=%d",
                    request.seed,
                    token_limit,
                )
                return upstream.tts_model.generate_voice_clone(
                    text=request.input,
                    language=voice_cfg.get("language", "Auto"),
                    ref_audio=voice_cfg["ref_audio"],
                    ref_text=voice_cfg.get("ref_text", ""),
                    max_new_tokens=token_limit,
                    instruct=request.instruct or None,
                )

        audio_arrays, sample_rate = await loop.run_in_executor(None, generate)
        audio = audio_arrays[0] if audio_arrays else upstream.np.zeros(1, dtype=upstream.np.float32)
        return Response(
            content=upstream._to_mp3_bytes(audio, sample_rate),
            media_type=content_types[output_format],
        )

    async def audio_stream():
        if output_format == "wav":
            yield upstream._wav_header(upstream.SAMPLE_RATE)
        async for raw_chunk in stream_chunks(
            voice_cfg, request.input, request.seed, request.instruct
        ):
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
