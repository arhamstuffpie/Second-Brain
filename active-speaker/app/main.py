from __future__ import annotations

import hmac
import importlib
import json
import logging
import os
import subprocess
import sys
import tempfile
import threading
from contextlib import asynccontextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from fastapi import FastAPI, File, Form, Header, HTTPException, UploadFile, status
from fastapi.concurrency import run_in_threadpool
from pydantic import BaseModel, Field, ValidationError

from app.evidence import build_evidence


logger = logging.getLogger("active-speaker")


@dataclass(frozen=True)
class Settings:
    model: str
    api_key: str
    runtime_path: Path
    work_dir: Path
    max_upload_bytes: int
    in_memory_frames: int

    @classmethod
    def from_environment(cls) -> "Settings":
        return cls(
            model=os.getenv("ACTIVE_SPEAKER_MODEL", "talknet/talkset-v1").strip(),
            api_key=os.getenv("ACTIVE_SPEAKER_API_KEY", "").strip(),
            runtime_path=Path(os.getenv("ACTIVE_SPEAKER_RUNTIME_PATH", "/opt/fast-asd/talknet")),
            work_dir=Path(os.getenv("ACTIVE_SPEAKER_WORK_DIR", "/tmp/active-speaker")),
            max_upload_bytes=int(os.getenv("ACTIVE_SPEAKER_MAX_UPLOAD_BYTES", str(512 * 1024 * 1024))),
            in_memory_frames=int(os.getenv("ACTIVE_SPEAKER_IN_MEMORY_FRAMES", "0")),
        )

    def validate(self) -> None:
        if not self.model or not self.runtime_path.is_dir():
            raise RuntimeError("TalkNet model name and runtime path are required")
        if self.max_upload_bytes < 1024 or self.in_memory_frames < 0:
            raise RuntimeError("active-speaker limits are invalid")


class PersonTrack(BaseModel):
    id: str = Field(min_length=1)
    start_time: float = Field(ge=0)
    end_time: float = Field(gt=0)
    tracking_confidence: float = Field(ge=0, le=1)
    evidence_frame_ids: list[str] = Field(default_factory=list)
    physical_presence: bool


class Segment(BaseModel):
    id: str = Field(min_length=1)
    start_time: float = Field(ge=0)
    end_time: float = Field(gt=0)
    speaker_profile_id: str = Field(min_length=1)


class Metadata(BaseModel):
    recording_id: str = Field(min_length=1)
    person_tracks: list[PersonTrack]
    segments: list[Segment]


class Evidence(BaseModel):
    person_track_id: str
    segment_ids: list[str]
    score: float
    visible_mouth_coverage: float
    overlapping_conflict: bool
    evidence_frame_ids: list[str]


class Analysis(BaseModel):
    provider: str
    model: str
    evidence: list[Evidence]
    warning: str = ""


class TalkNetRuntime:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self.lock = threading.Lock()
        sys.path.insert(0, str(settings.runtime_path))
        self.module = importlib.import_module("demoTalkNet")
        self.model, self.detector = self.module.setup()

    def analyze(self, video_path: Path) -> list[dict[str, Any]]:
        with self.lock, tempfile.TemporaryDirectory(dir=self.settings.work_dir) as job_dir:
            root = Path(job_dir)
            self.module.save_path = str(root)
            self.module.pyaviPath = str(root / "pyavi")
            self.module.pyframesPath = str(root / "pyframes")
            self.module.pyworkPath = str(root / "pywork")
            self.module.pycropPath = str(root / "pycrop")
            self.module.videoFilePath = str(root / "pyavi" / "video.avi")
            self.module.audioFilePath = str(root / "pyavi" / "audio.wav")
            return self.module.main(
                self.model,
                self.detector,
                str(video_path),
                return_visualization=False,
                in_memory_threshold=self.settings.in_memory_frames,
            )


settings = Settings.from_environment()


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings.validate()
    settings.work_dir.mkdir(parents=True, exist_ok=True)
    app.state.runtime = await run_in_threadpool(TalkNetRuntime, settings)
    yield
    app.state.runtime = None


app = FastAPI(
    title="TalkNet Active Speaker Service",
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
def healthz() -> dict[str, Any]:
    return {
        "status": "ok",
        "provider": "talknet",
        "model": settings.model,
        "version": "1",
        "details": {"runtime": "fast-asd"},
    }


@app.post("/v1/active-speakers", response_model=Analysis)
async def detect_active_speakers(
    file: UploadFile = File(...),
    model: str = Form(...),
    metadata: str = Form(...),
    authorization: str | None = Header(default=None),
) -> Analysis:
    authorize(authorization)
    if model.strip() != settings.model:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="unsupported model")
    try:
        parsed = Metadata.model_validate_json(metadata)
    except ValidationError as error:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=error.errors()) from error

    suffix = Path(file.filename or "recording.mp4").suffix or ".mp4"
    path = await save_upload(file, suffix)
    try:
        frames = await run_in_threadpool(app.state.runtime.analyze, path)
        fps = await run_in_threadpool(video_fps, path)
        evidence, warning = build_evidence(frames, json.loads(parsed.model_dump_json()), fps)
        return Analysis(provider="talknet", model=settings.model, evidence=evidence, warning=warning)
    except (RuntimeError, ValueError, subprocess.SubprocessError) as error:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=str(error)) from error
    except Exception as error:
        logger.exception("active-speaker inference failed")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="active-speaker inference failed",
        ) from error
    finally:
        path.unlink(missing_ok=True)


async def save_upload(upload: UploadFile, suffix: str) -> Path:
    descriptor, name = tempfile.mkstemp(prefix="active-speaker-", suffix=suffix, dir=settings.work_dir)
    path = Path(name)
    size = 0
    try:
        with os.fdopen(descriptor, "wb") as target:
            while chunk := await upload.read(1024 * 1024):
                size += len(chunk)
                if size > settings.max_upload_bytes:
                    raise HTTPException(
                        status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE,
                        detail="video file is too large",
                    )
                target.write(chunk)
        return path
    except Exception:
        path.unlink(missing_ok=True)
        raise


def video_fps(path: Path) -> float:
    output = subprocess.check_output(
        [
            "ffprobe", "-v", "error", "-select_streams", "v:0",
            "-show_entries", "stream=avg_frame_rate", "-of", "default=nw=1:nk=1", str(path),
        ],
        text=True,
        timeout=30,
    ).strip()
    numerator, separator, denominator = output.partition("/")
    fps = float(numerator) / float(denominator) if separator else float(numerator)
    if fps <= 0:
        raise ValueError("video frame rate is invalid")
    return fps
