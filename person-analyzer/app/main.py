from __future__ import annotations

import hashlib
import hmac
import json
import os
import platform
import tempfile
import threading
from contextlib import asynccontextmanager
from dataclasses import dataclass
from pathlib import Path

import cv2
import numpy as np
from fastapi import FastAPI, File, Form, Header, HTTPException, UploadFile, status
from fastapi.concurrency import run_in_threadpool
from pydantic import BaseModel, Field, model_validator

from app.tracker import DenseFaceTracker, Detection, TrackerConfig


@dataclass(frozen=True)
class Settings:
    detector_path: Path
    detector_sha256: str
    detector_id: str
    embedding_path: Path
    embedding_sha256: str
    embedding_id: str
    api_key: str
    device: str
    work_dir: Path
    max_upload_bytes: int
    max_duration_seconds: float
    default_fps: float
    min_face_pixels: int
    detection_threshold: float
    blur_threshold: float

    @classmethod
    def from_environment(cls) -> "Settings":
        return cls(
            detector_path=Path(os.getenv("PERSON_ANALYZER_DETECTOR_PATH", "/models/face_detection_yunet_2023mar.onnx")),
            detector_sha256=os.getenv("PERSON_ANALYZER_DETECTOR_SHA256", "8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4").lower(),
            detector_id=os.getenv("PERSON_ANALYZER_DETECTOR", "opencv/yunet-2023mar"),
            embedding_path=Path(os.getenv("PERSON_ANALYZER_EMBEDDING_PATH", "/models/face_recognition_sface_2021dec.onnx")),
            embedding_sha256=os.getenv("PERSON_ANALYZER_EMBEDDING_SHA256", "0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79").lower(),
            embedding_id=os.getenv("PERSON_ANALYZER_EMBEDDING_MODEL", "opencv/sface-2021dec"),
            api_key=os.getenv("PERSON_ANALYZER_API_KEY", "").strip(),
            device=os.getenv("PERSON_ANALYZER_DEVICE", "cpu").strip().lower(),
            work_dir=Path(os.getenv("PERSON_ANALYZER_WORK_DIR", "/tmp/person-analyzer")),
            max_upload_bytes=int(os.getenv("PERSON_ANALYZER_MAX_UPLOAD_BYTES", str(2 * 1024**3))),
            max_duration_seconds=float(os.getenv("PERSON_ANALYZER_MAX_DURATION_SECONDS", "14400")),
            default_fps=float(os.getenv("PERSON_ANALYZER_FPS", "8")),
            min_face_pixels=int(os.getenv("PERSON_ANALYZER_MIN_FACE_PIXELS", "64")),
            detection_threshold=float(os.getenv("PERSON_ANALYZER_DETECTION_THRESHOLD", "0.35")),
            blur_threshold=float(os.getenv("PERSON_ANALYZER_BLUR_THRESHOLD", "35")),
        )

    def validate(self) -> None:
        for name, digest in (("detector", self.detector_sha256), ("embedding", self.embedding_sha256)):
            if len(digest) != 64 or any(character not in "0123456789abcdef" for character in digest):
                raise RuntimeError(f"{name} SHA-256 is invalid")
        if self.device != "cpu":
            raise RuntimeError("PERSON_ANALYZER_DEVICE must be cpu; the CUDA image is not enabled yet")
        if self.max_upload_bytes < 1024 or self.max_duration_seconds <= 0 or not 0 < self.default_fps <= 30:
            raise RuntimeError("person analyzer limits are invalid")
        if self.min_face_pixels < 20 or not 0 < self.detection_threshold <= 1:
            raise RuntimeError("person analyzer face thresholds are invalid")


class TrackingProfile(BaseModel):
    fps: float = Field(default=8, gt=0, le=30)
    confirmation_detections: int = Field(default=3, ge=2, le=10)
    confirmation_window_frames: int = Field(default=5, ge=2, le=30)
    lost_timeout_seconds: float = Field(default=1, gt=0, le=30)
    reidentification_window_seconds: float = Field(default=10, gt=0, le=120)
    high_confidence_threshold: float = Field(default=0.8, ge=0, le=1)
    low_confidence_threshold: float = Field(default=0.35, ge=0, le=1)
    iou_threshold: float = Field(default=0.2, ge=0, le=1)
    appearance_threshold: float = Field(default=0.35, ge=-1, le=1)
    max_gallery_samples: int = Field(default=5, ge=1, le=20)

    @model_validator(mode="after")
    def valid_confidence_range(self) -> "TrackingProfile":
        if self.low_confidence_threshold > self.high_confidence_threshold:
            raise ValueError("low confidence threshold must not exceed high confidence threshold")
        return self


class RequestMetadata(BaseModel):
    recording_id: str = Field(min_length=1, max_length=200)
    processing_version: int = Field(gt=0)
    detector_model: str = Field(min_length=1)
    embedding_model: str = Field(min_length=1)
    profile: TrackingProfile = Field(default_factory=TrackingProfile)


class Box(BaseModel):
    x: int
    y: int
    width: int
    height: int


class Quality(BaseModel):
    usable: bool
    score: float
    reasons: list[str]


class Pose(BaseModel):
    yaw: float
    pitch: float
    roll: float
    bucket: str


class FaceObservation(BaseModel):
    observation_id: str
    frame_index: int
    timestamp: float
    box: Box
    landmarks: list[list[float]]
    detection_score: float
    quality: Quality
    pose: Pose
    embedding: list[float] | None = None
    mouth_visible: bool
    mouth_activity: float


class QualitySummary(BaseModel):
    mean: float
    maximum: float
    usable_observations: int


class PersonTrack(BaseModel):
    id: str
    provider_track_reference: str
    lifecycle_status: str
    first_frame: int
    last_frame: int
    start_time: float
    end_time: float
    observation_count: int
    tracking_confidence: float
    quality: QualitySummary
    gallery_observation_ids: list[str]
    observations: list[FaceObservation]


class Provenance(BaseModel):
    detector_model: str
    detector_checksum: str
    embedding_model: str
    embedding_checksum: str
    tracker: str
    runtime_version: str
    configuration_profile: dict[str, object]
    device: str


class AnalysisResponse(BaseModel):
    recording_id: str
    processing_version: int
    duration_seconds: float
    analyzed_fps: float
    tracks: list[PersonTrack]
    provenance: Provenance
    warnings: list[str]


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


class Analyzer:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        for name, path, expected in (
            ("detector", settings.detector_path, settings.detector_sha256),
            ("embedding", settings.embedding_path, settings.embedding_sha256),
        ):
            if not path.is_file():
                raise RuntimeError(f"configured {name} file does not exist: {path}")
            actual = file_sha256(path)
            if not hmac.compare_digest(actual, expected):
                raise RuntimeError(f"configured {name} SHA-256 mismatch: {actual}")
        self.detector = cv2.FaceDetectorYN.create(str(settings.detector_path), "", (320, 320), settings.detection_threshold, 0.3, 5000)
        self.recognizer = cv2.FaceRecognizerSF.create(str(settings.embedding_path), "")
        probe = np.asarray(self.recognizer.feature(np.zeros((112, 112, 3), dtype=np.uint8))).reshape(-1)
        if probe.size == 0 or not np.isfinite(probe).all():
            raise RuntimeError("SFace failed its startup probe")
        self.dimensions = int(probe.size)
        self.lock = threading.Lock()

    def analyze(self, video_path: Path, metadata: RequestMetadata) -> AnalysisResponse:
        capture = cv2.VideoCapture(str(video_path))
        if not capture.isOpened():
            raise ValueError("file must contain a decodable video")
        source_fps = float(capture.get(cv2.CAP_PROP_FPS))
        reported_frame_count = int(capture.get(cv2.CAP_PROP_FRAME_COUNT))
        if not np.isfinite(source_fps) or source_fps <= 0:
            capture.release()
            raise ValueError("video has invalid timing metadata")
        reported_duration = reported_frame_count/source_fps if reported_frame_count > 0 else 0
        if reported_duration > self.settings.max_duration_seconds:
            capture.release()
            raise ValueError("video exceeds the configured duration limit")
        profile = metadata.profile
        tracker = DenseFaceTracker(TrackerConfig(
            recording_id=metadata.recording_id,
            processing_version=metadata.processing_version,
            model_id=metadata.embedding_model,
            high_confidence=profile.high_confidence_threshold,
            low_confidence=profile.low_confidence_threshold,
            iou_threshold=profile.iou_threshold,
            appearance_threshold=profile.appearance_threshold,
            lost_timeout=profile.lost_timeout_seconds,
            reidentification_window=profile.reidentification_window_seconds,
            confirmation_detections=profile.confirmation_detections,
            confirmation_window_frames=profile.confirmation_window_frames,
        ))
        step = source_fps/profile.fps
        next_sample = 0.0
        frame_index = 0
        warnings: list[str] = []
        with self.lock:
            while True:
                ok, frame = capture.read()
                if not ok:
                    break
                timestamp = frame_index/source_fps
                if timestamp > self.settings.max_duration_seconds:
                    capture.release()
                    raise ValueError("video exceeds the configured duration limit")
                if frame_index >= next_sample:
                    tracker.update(frame_index, timestamp, self._detect(frame, frame_index, timestamp))
                    next_sample += max(step, 1.0)
                frame_index += 1
        capture.release()
        if frame_index == 0:
            raise ValueError("video contains no decodable frames")
        duration = frame_index/source_fps
        tracks = [self._serialize_track(track, profile.max_gallery_samples) for track in tracker.finish()]
        return AnalysisResponse(
            recording_id=metadata.recording_id,
            processing_version=metadata.processing_version,
            duration_seconds=round(duration, 6),
            analyzed_fps=profile.fps,
            tracks=tracks,
            provenance=Provenance(
                detector_model=self.settings.detector_id,
                detector_checksum=self.settings.detector_sha256,
                embedding_model=self.settings.embedding_id,
                embedding_checksum=self.settings.embedding_sha256,
                tracker="kalman-bytetrack-appearance-v1",
                runtime_version=f"python/{platform.python_version()} opencv/{cv2.__version__}",
                configuration_profile=profile.model_dump(),
                device=self.settings.device,
            ),
            warnings=warnings,
        )

    def _detect(self, frame: np.ndarray, frame_index: int, timestamp: float) -> list[Detection]:
        height, width = frame.shape[:2]
        self.detector.setInputSize((width, height))
        _, result = self.detector.detect(frame)
        detections: list[Detection] = []
        for row in [] if result is None else result:
            x, y, box_width, box_height = (float(value) for value in row[:4])
            landmarks = np.asarray(row[4:14], dtype=np.float64).reshape(5, 2)
            score = float(row[14])
            reasons, quality = self._quality(frame, x, y, box_width, box_height, score)
            embedding = None
            if not reasons:
                aligned = self.recognizer.alignCrop(frame, row)
                vector = np.asarray(self.recognizer.feature(aligned), dtype=np.float64).reshape(-1)
                norm = float(np.linalg.norm(vector))
                if vector.size and np.isfinite(vector).all() and norm > 0:
                    embedding = vector/norm
            detections.append(Detection(
                timestamp=timestamp,
                frame_index=frame_index,
                box=(x, y, box_width, box_height),
                landmarks=tuple((float(point[0]), float(point[1])) for point in landmarks),
                score=score,
                quality=quality,
                quality_reasons=tuple(reasons),
                pose=pose(landmarks),
                embedding=embedding,
            ))
        return detections

    def _quality(self, frame: np.ndarray, x: float, y: float, width: float, height: float, score: float) -> tuple[list[str], float]:
        reasons: list[str] = []
        image_height, image_width = frame.shape[:2]
        crop = frame[max(0, int(y)):min(image_height, int(y+height)), max(0, int(x)):min(image_width, int(x+width))]
        if width < self.settings.min_face_pixels or height < self.settings.min_face_pixels:
            reasons.append("face_too_small")
        if crop.size == 0:
            return [*reasons, "invalid_box"], 0.0
        gray = cv2.cvtColor(crop, cv2.COLOR_BGR2GRAY)
        sharpness = float(cv2.Laplacian(gray, cv2.CV_64F).var())
        if sharpness < self.settings.blur_threshold:
            reasons.append("blurred")
        brightness = float(gray.mean())
        if brightness < 35:
            reasons.append("underexposed")
        elif brightness > 220:
            reasons.append("overexposed")
        exposure = max(0.0, 1.0-abs(brightness-127.5)/127.5)
        quality = float(np.clip(score*0.5 + min(sharpness/200, 1)*0.3 + exposure*0.2, 0, 1))
        return reasons, round(quality, 4)

    @staticmethod
    def _serialize_track(track, maximum: int) -> PersonTrack:
        observations: list[FaceObservation] = []
        for item in track.observations:
            detection = item.detection
            observation_id = stable_observation_id(track.id, detection.frame_index, detection.timestamp)
            observations.append(FaceObservation(
                observation_id=observation_id,
                frame_index=detection.frame_index,
                timestamp=round(detection.timestamp, 6),
                box=Box(x=max(0, round(detection.box[0])), y=max(0, round(detection.box[1])), width=max(0, round(detection.box[2])), height=max(0, round(detection.box[3]))),
                landmarks=[[round(value, 3) for value in point] for point in detection.landmarks],
                detection_score=round(detection.score, 4),
                quality=Quality(usable=not detection.quality_reasons, score=detection.quality, reasons=list(detection.quality_reasons)),
                pose=Pose(yaw=detection.pose[0], pitch=detection.pose[1], roll=detection.pose[2], bucket=detection.pose[3]),
                embedding=detection.embedding.tolist() if detection.embedding is not None else None,
                mouth_visible=item.mouth_visible,
                mouth_activity=item.mouth_activity,
            ))
        eligible = [item for item in observations if item.quality.usable and item.embedding]
        gallery = select_gallery(eligible, maximum)
        qualities = [item.quality.score for item in observations]
        return PersonTrack(
            id=track.id,
            provider_track_reference=track.id,
            lifecycle_status=track.lifecycle.value,
            first_frame=observations[0].frame_index,
            last_frame=observations[-1].frame_index,
            start_time=observations[0].timestamp,
            end_time=observations[-1].timestamp,
            observation_count=len(observations),
            tracking_confidence=round(min(item.detection_score for item in track.observations), 4),
            quality=QualitySummary(mean=round(sum(qualities)/len(qualities), 4), maximum=max(qualities), usable_observations=len(eligible)),
            gallery_observation_ids=[item.observation_id for item in gallery],
            observations=observations,
        )


def pose(landmarks: np.ndarray) -> tuple[float, float, float, str]:
    left_eye, right_eye, nose, left_mouth, right_mouth = landmarks
    eye_midpoint = (left_eye+right_eye)/2
    mouth_midpoint = (left_mouth+right_mouth)/2
    eye_distance = max(float(np.linalg.norm(right_eye-left_eye)), 1.0)
    face_height = max(float(np.linalg.norm(mouth_midpoint-eye_midpoint)), 1.0)
    yaw = float(np.degrees(np.arcsin(np.clip((nose[0]-eye_midpoint[0])/(eye_distance/2), -1, 1))))
    pitch = float(np.clip(((nose[1]-eye_midpoint[1])/face_height-0.55)*90, -90, 90))
    delta = right_eye-left_eye
    roll = float(np.degrees(np.arctan2(delta[1], delta[0])))
    absolute = abs(yaw)
    bucket = "frontal" if absolute < 20 else ("right_three_quarter" if yaw > 0 else "left_three_quarter") if absolute < 45 else ("right_profile" if yaw > 0 else "left_profile")
    return round(yaw, 2), round(pitch, 2), round(roll, 2), bucket


def pose_priority(bucket: str) -> int:
    return {"frontal": 0, "left_three_quarter": 1, "right_three_quarter": 1, "left_profile": 2, "right_profile": 2}.get(bucket, 3)


def select_gallery(observations: list[FaceObservation], maximum: int) -> list[FaceObservation]:
    ordered = sorted(observations, key=lambda item: (-item.quality.score, item.timestamp))
    selected: list[FaceObservation] = []
    for bucket in ("frontal", "left_three_quarter", "right_three_quarter", "left_profile", "right_profile"):
        if match := next((item for item in ordered if item.pose.bucket == bucket), None):
            selected.append(match)
            if len(selected) == maximum:
                return selected
    selected_ids = {item.observation_id for item in selected}
    selected.extend(item for item in ordered if item.observation_id not in selected_ids)
    return selected[:maximum]


def stable_observation_id(track_id: str, frame_index: int, timestamp: float) -> str:
    source = f"{track_id}\0{frame_index}\0{timestamp:.6f}".encode()
    return "face-observation-" + hashlib.sha256(source).hexdigest()[:32]


settings = Settings.from_environment()
settings.validate()


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings.work_dir.mkdir(parents=True, exist_ok=True)
    app.state.analyzer = await run_in_threadpool(Analyzer, settings)
    yield
    app.state.analyzer = None


app = FastAPI(title="Dense Person Analyzer", version="1.0.0", docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)


def authorize(authorization: str | None) -> None:
    if settings.api_key and (authorization is None or not hmac.compare_digest(authorization, f"Bearer {settings.api_key}")):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid API key")


def health() -> dict[str, object]:
    return {
        "status": "ok",
        "detector_model": settings.detector_id,
        "detector_checksum": settings.detector_sha256,
        "embedding_model": settings.embedding_id,
        "embedding_checksum": settings.embedding_sha256,
        "tracker": "kalman-bytetrack-appearance-v1",
        "runtime_version": f"python/{platform.python_version()} opencv/{cv2.__version__}",
        "device": settings.device,
        "dimensions": app.state.analyzer.dimensions,
    }


@app.get("/healthz")
def healthz() -> dict[str, object]:
    return health()


@app.get("/readyz")
def readyz() -> dict[str, object]:
    if app.state.analyzer is None:
        raise HTTPException(status_code=status.HTTP_503_SERVICE_UNAVAILABLE, detail="models are not loaded")
    return health()


@app.post("/v1/person-tracks", response_model=AnalysisResponse)
async def create_person_tracks(
    file: UploadFile = File(...),
    metadata: str = Form(...),
    authorization: str | None = Header(default=None),
) -> AnalysisResponse:
    authorize(authorization)
    try:
        request = RequestMetadata.model_validate_json(metadata)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=f"invalid metadata: {error}") from error
    if request.detector_model != settings.detector_id or request.embedding_model != settings.embedding_id:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="requested model IDs do not match loaded models")
    suffix = Path(file.filename or "video.mp4").suffix[:10]
    total = 0
    try:
        with tempfile.NamedTemporaryFile(dir=settings.work_dir, suffix=suffix, delete=False) as target:
            path = Path(target.name)
            while chunk := await file.read(1024 * 1024):
                total += len(chunk)
                if total > settings.max_upload_bytes:
                    raise HTTPException(status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE, detail="video file is too large")
                target.write(chunk)
        if total == 0:
            raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="video file is empty")
        return await run_in_threadpool(app.state.analyzer.analyze, path, request)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=str(error)) from error
    finally:
        if "path" in locals():
            path.unlink(missing_ok=True)
