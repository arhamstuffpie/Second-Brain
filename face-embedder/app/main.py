from __future__ import annotations

import hashlib
import hmac
import os
import threading
from contextlib import asynccontextmanager
from dataclasses import dataclass
from pathlib import Path

import cv2
import numpy as np
from fastapi import FastAPI, File, Form, Header, HTTPException, UploadFile, status
from fastapi.concurrency import run_in_threadpool
from pydantic import BaseModel


@dataclass(frozen=True)
class Settings:
    detector_path: Path
    detector_sha256: str
    detector_id: str
    model_path: Path
    model_sha256: str
    model: str
    api_key: str
    max_upload_bytes: int
    max_image_pixels: int
    min_face_pixels: int
    detector_score: float
    blur_threshold: float

    @classmethod
    def from_environment(cls) -> "Settings":
        return cls(
            detector_path=Path(os.getenv("FACE_EMBEDDER_DETECTOR_PATH", "/models/face_detection_yunet_2023mar.onnx")),
            detector_sha256=os.getenv("FACE_EMBEDDER_DETECTOR_SHA256", "8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4").strip().lower(),
            detector_id=os.getenv("FACE_EMBEDDER_DETECTOR", "opencv/yunet-2023mar").strip(),
            model_path=Path(os.getenv("FACE_EMBEDDER_MODEL_PATH", "/models/face_recognition_sface_2021dec.onnx")),
            model_sha256=os.getenv("FACE_EMBEDDER_MODEL_SHA256", "0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79").strip().lower(),
            model=os.getenv("FACE_EMBEDDER_MODEL", "opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79").strip(),
            api_key=os.getenv("FACE_EMBEDDER_API_KEY", "").strip(),
            max_upload_bytes=int(os.getenv("FACE_EMBEDDER_MAX_UPLOAD_BYTES", str(10 * 1024 * 1024))),
            max_image_pixels=int(os.getenv("FACE_EMBEDDER_MAX_IMAGE_PIXELS", "16000000")),
            min_face_pixels=int(os.getenv("FACE_EMBEDDER_MIN_FACE_PIXELS", "64")),
            detector_score=float(os.getenv("FACE_EMBEDDER_DETECTION_THRESHOLD", "0.80")),
            blur_threshold=float(os.getenv("FACE_EMBEDDER_BLUR_THRESHOLD", "50")),
        )

    def validate(self) -> None:
        for name, value in (("detector", self.detector_sha256), ("model", self.model_sha256)):
            if len(value) != 64 or any(character not in "0123456789abcdef" for character in value):
                raise RuntimeError(f"{name} SHA-256 is invalid")
        if not self.detector_id or not self.model:
            raise RuntimeError("detector and embedding model identifiers are required")
        if self.max_upload_bytes < 1024 or self.max_image_pixels < 4096 or self.min_face_pixels < 20:
            raise RuntimeError("face image limits are invalid")
        if not 0 < self.detector_score <= 1 or self.blur_threshold < 0:
            raise RuntimeError("face quality thresholds are invalid")


class Box(BaseModel):
    x: int
    y: int
    width: int
    height: int


class Quality(BaseModel):
    usable: bool
    reasons: list[str]


class Face(BaseModel):
    box: Box
    landmarks: list[list[float]]
    detection_score: float
    quality: Quality
    embedding: list[float] | None = None


class EmbeddingResponse(BaseModel):
    provider: str
    detector: str
    model: str
    dimensions: int
    faces: list[Face]


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


class Embedder:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        for name, path, expected in (
            ("detector", settings.detector_path, settings.detector_sha256),
            ("model", settings.model_path, settings.model_sha256),
        ):
            if not path.is_file():
                raise RuntimeError(f"configured {name} file does not exist: {path}")
            actual = file_sha256(path)
            if not hmac.compare_digest(actual, expected):
                raise RuntimeError(f"configured {name} SHA-256 mismatch: {actual}")
        self.detector = cv2.FaceDetectorYN.create(str(settings.detector_path), "", (320, 320), settings.detector_score, 0.3, 5000)
        self.recognizer = cv2.FaceRecognizerSF.create(str(settings.model_path), "")
        self.lock = threading.Lock()
        self.dimensions = self._startup_probe()

    def _startup_probe(self) -> int:
        image = np.zeros((112, 112, 3), dtype=np.uint8)
        self.detector.setInputSize((112, 112))
        self.detector.detect(image)
        vector = np.asarray(self.recognizer.feature(image)).reshape(-1)
        if vector.size == 0 or not np.isfinite(vector).all():
            raise RuntimeError("face embedding model failed its startup probe")
        return int(vector.size)

    def encode(self, payload: bytes, single_face: bool) -> list[Face]:
        encoded = np.frombuffer(payload, dtype=np.uint8)
        image = cv2.imdecode(encoded, cv2.IMREAD_COLOR)
        if image is None or image.ndim != 3 or image.shape[2] != 3:
            raise ValueError("file must contain a decodable JPEG, PNG, or WebP image")
        height, width = image.shape[:2]
        if height * width > self.settings.max_image_pixels:
            raise ValueError("decoded image exceeds the configured pixel limit")
        with self.lock:
            self.detector.setInputSize((width, height))
            _, detections = self.detector.detect(image)
            rows = [] if detections is None else detections
            if single_face and len(rows) != 1:
                raise ValueError("enrollment requires exactly one detected face")
            return [self._face(image, row) for row in rows]

    def _face(self, image: np.ndarray, row: np.ndarray) -> Face:
        x, y, width, height = (int(round(float(value))) for value in row[:4])
        landmarks = np.asarray(row[4:14], dtype=np.float64).reshape(5, 2)
        score = float(row[14])
        reasons = self._quality_reasons(image, x, y, width, height, landmarks, score)
        embedding: list[float] | None = None
        if not reasons:
            aligned = self.recognizer.alignCrop(image, row)
            vector = np.asarray(self.recognizer.feature(aligned), dtype=np.float64).reshape(-1)
            norm = float(np.linalg.norm(vector))
            if vector.size != self.dimensions or not np.isfinite(vector).all() or norm <= 0:
                raise RuntimeError("model produced an invalid face embedding")
            embedding = (vector / norm).tolist()
        return Face(
            box=Box(x=max(0, x), y=max(0, y), width=max(0, width), height=max(0, height)),
            landmarks=landmarks.tolist(), detection_score=score,
            quality=Quality(usable=not reasons, reasons=reasons), embedding=embedding,
        )

    def _quality_reasons(
        self, image: np.ndarray, x: int, y: int, width: int, height: int,
        landmarks: np.ndarray, score: float,
    ) -> list[str]:
        reasons: list[str] = []
        image_height, image_width = image.shape[:2]
        left, top = max(0, x), max(0, y)
        right, bottom = min(image_width, x + width), min(image_height, y + height)
        crop = image[top:bottom, left:right]
        if score < self.settings.detector_score:
            reasons.append("low_detection_confidence")
        if width < self.settings.min_face_pixels or height < self.settings.min_face_pixels:
            reasons.append("face_too_small")
        if crop.size == 0 or not np.isfinite(landmarks).all():
            reasons.append("invalid_landmarks")
            return reasons
        gray = cv2.cvtColor(crop, cv2.COLOR_BGR2GRAY)
        if float(cv2.Laplacian(gray, cv2.CV_64F).var()) < self.settings.blur_threshold:
            reasons.append("blurred")
        brightness = float(gray.mean())
        if brightness < 35:
            reasons.append("underexposed")
        elif brightness > 220:
            reasons.append("overexposed")
        eye_distance = float(np.linalg.norm(landmarks[1] - landmarks[0]))
        if eye_distance < max(8.0, width * 0.18):
            reasons.append("unreliable_landmarks")
        else:
            eye_delta = landmarks[1] - landmarks[0]
            eye_angle = abs(float(np.degrees(np.arctan2(eye_delta[1], eye_delta[0]))))
            if eye_angle > 30:
                reasons.append("severe_roll")
            eye_midpoint = (landmarks[0] + landmarks[1]) / 2
            if abs(float(landmarks[2, 0] - eye_midpoint[0])) > eye_distance * 0.45:
                reasons.append("severe_yaw")
        return reasons


settings = Settings.from_environment()
settings.validate()


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.embedder = await run_in_threadpool(Embedder, settings)
    yield
    app.state.embedder = None


app = FastAPI(
    title="YuNet/SFace Face Embedder", version="1.0.0", docs_url=None,
    redoc_url=None, openapi_url=None, lifespan=lifespan,
)


def authorize(authorization: str | None) -> None:
    if not settings.api_key:
        return
    expected = f"Bearer {settings.api_key}"
    if authorization is None or not hmac.compare_digest(authorization, expected):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid API key")


@app.get("/healthz")
def healthz() -> dict[str, str | int]:
    embedder: Embedder = app.state.embedder
    return {
        "status": "ok", "provider": "opencv", "detector": settings.detector_id,
        "detector_sha256": settings.detector_sha256, "model": settings.model,
        "model_sha256": settings.model_sha256, "dimensions": embedder.dimensions,
    }


@app.post("/v1/embeddings", response_model=EmbeddingResponse)
async def create_embeddings(
    file: UploadFile = File(...), model: str = Form(...),
    single_face: bool = Form(False), authorization: str | None = Header(default=None),
) -> EmbeddingResponse:
    authorize(authorization)
    if model.strip() != settings.model:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="unsupported model")
    payload = await file.read(settings.max_upload_bytes + 1)
    if not payload:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="image file is empty")
    if len(payload) > settings.max_upload_bytes:
        raise HTTPException(status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE, detail="image file is too large")
    try:
        faces = await run_in_threadpool(app.state.embedder.encode, payload, single_face)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=str(error)) from error
    return EmbeddingResponse(
        provider="opencv", detector=settings.detector_id, model=settings.model,
        dimensions=app.state.embedder.dimensions, faces=faces,
    )
