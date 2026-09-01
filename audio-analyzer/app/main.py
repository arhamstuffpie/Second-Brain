from __future__ import annotations

import base64
import hashlib
import hmac
import io
import os
import platform
import subprocess
import tempfile
import threading
import time
from contextlib import asynccontextmanager
from dataclasses import dataclass
from pathlib import Path

import numpy as np
import soundfile as sf
import torch
from fastapi import FastAPI, File, Form, Header, HTTPException, UploadFile, status
from fastapi.concurrency import run_in_threadpool
from pyannote.audio import Pipeline
from pydantic import BaseModel, Field
from speechbrain.pretrained import SepformerSeparation

from app.timeline import Region, Turn, build_regions, stable_source_id


@dataclass(frozen=True)
class Settings:
    diarization_path: Path
    diarization_id: str
    separation_path: Path
    separation_id: str
    manifest_path: Path
    api_key: str
    device: str
    work_dir: Path
    max_upload_bytes: int

    @classmethod
    def from_environment(cls) -> "Settings":
        return cls(
            diarization_path=Path(os.getenv("AUDIO_ANALYZER_DIARIZATION_PATH", "/models/pyannote-community-1")),
            diarization_id=os.getenv("AUDIO_ANALYZER_DIARIZATION_MODEL", "pyannote/speaker-diarization-community-1"),
            separation_path=Path(os.getenv("AUDIO_ANALYZER_SEPARATION_PATH", "/models/sepformer-whamr16k")),
            separation_id=os.getenv("AUDIO_ANALYZER_SEPARATION_MODEL", "speechbrain/sepformer-whamr16k@cfd5df650d52eb60ba93e009acdb9894c5900038"),
            manifest_path=Path(os.getenv("AUDIO_ANALYZER_MANIFEST", "/models/SHA256SUMS")),
            api_key=os.getenv("AUDIO_ANALYZER_API_KEY", "").strip(),
            device=os.getenv("AUDIO_ANALYZER_DEVICE", "cpu").strip().lower(),
            work_dir=Path(os.getenv("AUDIO_ANALYZER_WORK_DIR", "/tmp/audio-analyzer")),
            max_upload_bytes=int(os.getenv("AUDIO_ANALYZER_MAX_UPLOAD_BYTES", str(1024**3))),
        )

    def validate(self) -> None:
        if self.device not in ("cpu", "cuda"):
            raise RuntimeError("AUDIO_ANALYZER_DEVICE must be cpu or cuda")
        if self.max_upload_bytes < 1024 or not self.diarization_id or not self.separation_id:
            raise RuntimeError("audio analyzer configuration is invalid")


class Profile(BaseModel):
    maximum_speakers: int = Field(default=4, ge=1, le=8)
    maximum_overlap_window_seconds: float = Field(default=30, gt=0, le=120)
    separation_budget_seconds: float = Field(default=300, ge=0, le=3600)
    speaker_match_threshold: float = Field(default=0.62, ge=-1, le=1)
    speaker_match_margin: float = Field(default=0.08, ge=0, le=1)


class Metadata(BaseModel):
    recording_id: str = Field(min_length=1, max_length=200)
    processing_version: int = Field(gt=0)
    profile: Profile = Field(default_factory=Profile)


class AudioRegion(BaseModel):
    id: str
    start_time: float
    end_time: float
    kind: str
    active_speaker_labels: list[str]
    concurrent_speaker_count: int
    overlap: bool
    diarization_confidence: float | None = None
    status: str


class AudioSource(BaseModel):
    id: str
    audio_region_id: str
    source_index: int
    diarization_cluster_id: str = ""
    separation_status: str
    separation_confidence: float | None = None
    reconstruction_score: float | None = None
    speaker_match_score: float | None = None
    speaker_runner_up_score: float | None = None
    audio_base64: str = ""


class Provenance(BaseModel):
    diarization_model: str
    diarization_checksum: str
    separation_model: str
    separation_checksum: str
    runtime_version: str
    configuration_profile: dict[str, object]
    device: str


class Analysis(BaseModel):
    recording_id: str
    processing_version: int
    duration_seconds: float
    regions: list[AudioRegion]
    sources: list[AudioSource]
    provenance: Provenance
    warnings: list[str]


def verify_manifest(path: Path, root: Path) -> str:
    if not path.is_file():
        raise RuntimeError(f"model checksum manifest does not exist: {path}")
    content = path.read_bytes()
    for line in content.decode().splitlines():
        expected, relative = line.split("  ", 1)
        target = (root / relative).resolve()
        if root.resolve() not in target.parents or not target.is_file():
            raise RuntimeError(f"model manifest entry is invalid: {relative}")
        actual = hashlib.sha256(target.read_bytes()).hexdigest()
        if not hmac.compare_digest(actual, expected):
            raise RuntimeError(f"model checksum mismatch: {relative}")
    return hashlib.sha256(content).hexdigest()


class Runtime:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self.manifest_checksum = verify_manifest(settings.manifest_path, settings.manifest_path.parent)
        if not settings.diarization_path.is_dir() or not settings.separation_path.is_dir():
            raise RuntimeError("offline diarization and separation model directories are required")
        self.diarization = Pipeline.from_pretrained(str(settings.diarization_path))
        if settings.device == "cuda":
            self.diarization.to(torch.device("cuda"))
        self.separator = SepformerSeparation.from_hparams(
            source=str(settings.separation_path),
            savedir=str(settings.work_dir / "sepformer-runtime"),
            run_opts={"device": settings.device},
        )
        self.lock = threading.Lock()

    def analyze(self, normalized_path: Path, metadata: Metadata) -> Analysis:
        waveform, sample_rate = sf.read(normalized_path, dtype="float32", always_2d=False)
        if waveform.ndim != 1 or sample_rate != 16000:
            raise ValueError("normalized input must be mono PCM at 16 kHz")
        duration = len(waveform)/sample_rate
        with self.lock:
            output = self.diarization(str(normalized_path), max_speakers=metadata.profile.maximum_speakers)
        turns = [Turn(float(turn.start), float(turn.end), str(speaker)) for turn, _, speaker in output.speaker_diarization.itertracks(yield_label=True)]
        regions = build_regions(metadata.recording_id, metadata.processing_version, turns, duration)
        deadline = time.monotonic() + metadata.profile.separation_budget_seconds
        response_regions: list[AudioRegion] = []
        sources: list[AudioSource] = []
        warnings: list[str] = []
        for region in regions:
            region_status = "completed"
            if region.kind == "speech":
                sources.append(AudioSource(
                    id=stable_source_id(region.id, 0), audio_region_id=region.id, source_index=0,
                    diarization_cluster_id=region.speakers[0], separation_status="not_required",
                ))
            elif region.kind == "overlap":
                if time.monotonic() >= deadline:
                    region_status = "budget_exhausted"
                    warnings.append(f"separation budget exhausted at {region.start:.3f}s")
                else:
                    separated, reconstruction = self._separate_region(
                        waveform, sample_rate, region, metadata.profile, deadline,
                    )
                    if not separated:
                        region_status = "ambiguous"
                    for index, source_audio in enumerate(separated):
                        sources.append(AudioSource(
                            id=stable_source_id(region.id, index), audio_region_id=region.id,
                            source_index=index, separation_status="ambiguous",
                            reconstruction_score=reconstruction,
                            separation_confidence=reconstruction,
                            audio_base64=encode_wav(source_audio, sample_rate),
                        ))
                    warnings.append(f"overlap {region.id} was separated but remains identity-ambiguous")
            response_regions.append(AudioRegion(
                id=region.id, start_time=region.start, end_time=region.end, kind=region.kind,
                active_speaker_labels=list(region.speakers), concurrent_speaker_count=len(region.speakers),
                overlap=region.kind == "overlap", status=region_status,
            ))
        return Analysis(
            recording_id=metadata.recording_id,
            processing_version=metadata.processing_version,
            duration_seconds=duration,
            regions=response_regions,
            sources=sources,
            provenance=Provenance(
                diarization_model=self.settings.diarization_id,
                diarization_checksum=self.manifest_checksum,
                separation_model=self.settings.separation_id,
                separation_checksum=self.manifest_checksum,
                runtime_version=f"python/{platform.python_version()} torch/{torch.__version__}",
                configuration_profile=metadata.profile.model_dump(),
                device=self.settings.device,
            ),
            warnings=warnings,
        )

    def _separate_region(self, waveform: np.ndarray, rate: int, region: Region, profile: Profile, deadline: float) -> tuple[list[np.ndarray], float]:
        start, end = round(region.start*rate), round(region.end*rate)
        mixture = waveform[start:end]
        if len(mixture) == 0 or (region.end-region.start) > profile.maximum_overlap_window_seconds:
            return [], 0.0
        desired = min(len(region.speakers), profile.maximum_speakers)
        pending = [mixture]
        separated: list[np.ndarray] = []
        while pending and len(separated)+len(pending) < desired and time.monotonic() < deadline:
            current = pending.pop()
            with tempfile.NamedTemporaryFile(dir=self.settings.work_dir, suffix=".wav") as source:
                sf.write(source.name, current, rate, subtype="PCM_16")
                with self.lock:
                    result = self.separator.separate_file(source.name).detach().cpu().numpy()
            result = np.squeeze(result, axis=0) if result.ndim == 3 else result
            if result.ndim != 2 or result.shape[1] < 2:
                separated.append(current)
                continue
            separated.append(result[:, 0])
            pending.append(result[:, 1])
        separated.extend(pending)
        separated = [fit_length(item, len(mixture)) for item in separated[:desired]]
        if len(separated) < 2:
            return [], 0.0
        reconstruction = float(np.clip(1-np.linalg.norm(mixture-np.sum(separated, axis=0))/(np.linalg.norm(mixture)+1e-8), 0, 1))
        return separated, round(reconstruction, 4)


def fit_length(audio: np.ndarray, length: int) -> np.ndarray:
    result = np.zeros(length, dtype=np.float32)
    result[:min(length, len(audio))] = np.asarray(audio[:length], dtype=np.float32)
    return result


def encode_wav(audio: np.ndarray, rate: int) -> str:
    target = io.BytesIO()
    sf.write(target, audio, rate, format="WAV", subtype="PCM_16")
    return base64.b64encode(target.getvalue()).decode()


def normalize_audio(source: Path, target: Path) -> None:
    result = subprocess.run(
        ["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", str(source), "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", str(target)],
        capture_output=True, text=True, timeout=300, check=False,
    )
    if result.returncode != 0:
        raise ValueError("audio could not be normalized: " + result.stderr.strip()[:500])


settings = Settings.from_environment()
settings.validate()


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings.work_dir.mkdir(parents=True, exist_ok=True)
    app.state.runtime = await run_in_threadpool(Runtime, settings)
    yield
    app.state.runtime = None


app = FastAPI(title="Overlap-aware Audio Analyzer", version="1.0.0", docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)


def authorize(authorization: str | None) -> None:
    if settings.api_key and (authorization is None or not hmac.compare_digest(authorization, f"Bearer {settings.api_key}")):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid API key")


def health() -> dict[str, object]:
    runtime: Runtime = app.state.runtime
    return {
        "status": "ok", "diarization_model": settings.diarization_id,
        "diarization_checksum": runtime.manifest_checksum,
        "separation_model": settings.separation_id, "separation_checksum": runtime.manifest_checksum,
        "runtime_version": f"python/{platform.python_version()} torch/{torch.__version__}",
        "device": settings.device, "supported_overlap_count": 4,
    }


@app.get("/healthz")
def healthz() -> dict[str, object]:
    return health()


@app.get("/readyz")
def readyz() -> dict[str, object]:
    if app.state.runtime is None:
        raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="models are not loaded")
    return health()


@app.post("/v1/audio-analysis", response_model=Analysis)
async def analyze_audio(
    file: UploadFile = File(...), metadata: str = Form(...),
    authorization: str | None = Header(default=None),
) -> Analysis:
    authorize(authorization)
    try:
        request = Metadata.model_validate_json(metadata)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=f"invalid metadata: {error}") from error
    total = 0
    try:
        with tempfile.NamedTemporaryFile(dir=settings.work_dir, suffix=Path(file.filename or "audio").suffix[:10], delete=False) as source:
            source_path = Path(source.name)
            while chunk := await file.read(1024*1024):
                total += len(chunk)
                if total > settings.max_upload_bytes:
                    raise HTTPException(status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE, detail="audio file is too large")
                source.write(chunk)
        if total == 0:
            raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="audio file is empty")
        normalized_path = settings.work_dir / f"{source_path.name}.wav"
        await run_in_threadpool(normalize_audio, source_path, normalized_path)
        return await run_in_threadpool(app.state.runtime.analyze, normalized_path, request)
    except (ValueError, subprocess.TimeoutExpired) as error:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=str(error)) from error
    finally:
        if "source_path" in locals():
            source_path.unlink(missing_ok=True)
        if "normalized_path" in locals():
            normalized_path.unlink(missing_ok=True)
