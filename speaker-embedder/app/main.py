from __future__ import annotations

import hmac
import inspect
import io
import os
import threading
from contextlib import asynccontextmanager
from dataclasses import dataclass
from pathlib import Path

import numpy as np
import soundfile as sf
import torch
import torchaudio.functional as audio_functional
import huggingface_hub
from fastapi import FastAPI, File, Form, Header, HTTPException, UploadFile, status
from fastapi.concurrency import run_in_threadpool
from pydantic import BaseModel

if "use_auth_token" not in inspect.signature(huggingface_hub.hf_hub_download).parameters:
    _hf_hub_download = huggingface_hub.hf_hub_download

    def hf_hub_download_compatible(*args, use_auth_token=None, **kwargs):
        if use_auth_token is not None and "token" not in kwargs:
            kwargs["token"] = use_auth_token
        return _hf_hub_download(*args, **kwargs)

    huggingface_hub.hf_hub_download = hf_hub_download_compatible

from speechbrain.inference.speaker import EncoderClassifier


@dataclass(frozen=True)
class Settings:
    model: str
    cache_dir: str
    api_key: str
    max_upload_bytes: int
    min_duration_seconds: float
    max_duration_seconds: float

    @classmethod
    def from_environment(cls) -> "Settings":
        return cls(
            model=os.getenv("SPEAKER_EMBEDDER_MODEL", "speechbrain/spkrec-ecapa-voxceleb").strip(),
            cache_dir=os.getenv("SPEAKER_EMBEDDER_CACHE_DIR", "/var/cache/speaker-embedder").strip(),
            api_key=os.getenv("SPEAKER_EMBEDDER_API_KEY", "").strip(),
            max_upload_bytes=int(os.getenv("SPEAKER_EMBEDDER_MAX_UPLOAD_BYTES", str(5 * 1024 * 1024))),
            min_duration_seconds=float(os.getenv("SPEAKER_EMBEDDER_MIN_DURATION_SECONDS", "2")),
            max_duration_seconds=float(os.getenv("SPEAKER_EMBEDDER_MAX_DURATION_SECONDS", "10")),
        )

    def validate(self) -> None:
        if not self.model or not self.cache_dir:
            raise RuntimeError("speaker model and cache directory are required")
        if self.max_upload_bytes < 1024:
            raise RuntimeError("maximum upload size is invalid")
        if self.min_duration_seconds < 2 or not self.min_duration_seconds <= self.max_duration_seconds <= 10:
            raise RuntimeError("speaker audio duration must be between 2 and 10 seconds")


class EmbeddingResponse(BaseModel):
    embedding: list[float]
    model: str
    dimensions: int


class Embedder:
    def __init__(self, settings: Settings) -> None:
        Path(settings.cache_dir).mkdir(parents=True, exist_ok=True)
        self.settings = settings
        self.classifier = EncoderClassifier.from_hparams(
            source=settings.model,
            savedir=str(Path(settings.cache_dir) / "model"),
            run_opts={"device": "cpu"},
        )
        self.lock = threading.Lock()

    def encode(self, payload: bytes) -> np.ndarray:
        try:
            waveform, sample_rate = sf.read(io.BytesIO(payload), dtype="float32", always_2d=True)
        except (RuntimeError, ValueError) as error:
            raise ValueError("file must contain decodable WAV or FLAC audio") from error
        if waveform.shape[0] == 0 or sample_rate <= 0:
            raise ValueError("audio is empty")
        mono = torch.from_numpy(waveform.mean(axis=1)).unsqueeze(0)
        duration = mono.shape[1] / float(sample_rate)
        if duration < self.settings.min_duration_seconds or duration > self.settings.max_duration_seconds + 0.05:
            raise ValueError(
                f"audio duration must be between {self.settings.min_duration_seconds:g} "
                f"and {self.settings.max_duration_seconds:g} seconds"
            )
        if sample_rate != 16_000:
            mono = audio_functional.resample(mono, sample_rate, 16_000)
        with self.lock, torch.inference_mode():
            vector = self.classifier.encode_batch(mono, normalize=True).squeeze().cpu().numpy()
        if vector.ndim != 1 or vector.size == 0 or not np.isfinite(vector).all():
            raise RuntimeError("model produced an invalid embedding")
        magnitude = float(np.linalg.norm(vector))
        if magnitude <= 0:
            raise RuntimeError("model produced a zero-magnitude embedding")
        return vector.astype(np.float64) / magnitude


settings = Settings.from_environment()
settings.validate()


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.embedder = await run_in_threadpool(Embedder, settings)
    yield
    app.state.embedder = None


app = FastAPI(
    title="ECAPA Speaker Embedder",
    version="1.0.0",
    docs_url=None,
    redoc_url=None,
    openapi_url=None,
    lifespan=lifespan,
)


def authorize(authorization: str | None) -> None:
    if not settings.api_key:
        return
    expected = f"Bearer {settings.api_key}"
    if authorization is None or not hmac.compare_digest(authorization, expected):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid API key")


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok", "model": settings.model}


@app.post("/v1/embeddings", response_model=EmbeddingResponse)
async def create_embedding(
    file: UploadFile = File(...),
    model: str = Form(...),
    authorization: str | None = Header(default=None),
) -> EmbeddingResponse:
    authorize(authorization)
    if model.strip() != settings.model:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="unsupported model")
    payload = await file.read(settings.max_upload_bytes + 1)
    if len(payload) > settings.max_upload_bytes:
        raise HTTPException(status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE, detail="audio file is too large")
    try:
        vector = await run_in_threadpool(app.state.embedder.encode, payload)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=str(error)) from error
    return EmbeddingResponse(
        embedding=vector.tolist(),
        model=settings.model,
        dimensions=int(vector.size),
    )
