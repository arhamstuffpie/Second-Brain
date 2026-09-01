from __future__ import annotations

import hashlib
import math
from dataclasses import dataclass, field
from enum import Enum

import cv2
import numpy as np
from scipy.optimize import linear_sum_assignment


class Lifecycle(str, Enum):
    TENTATIVE = "tentative"
    CONFIRMED = "confirmed"
    LOST = "lost"
    ENDED = "ended"


@dataclass(frozen=True)
class Detection:
    timestamp: float
    frame_index: int
    box: tuple[float, float, float, float]
    landmarks: tuple[tuple[float, float], ...]
    score: float
    quality: float
    quality_reasons: tuple[str, ...]
    pose: tuple[float, float, float, str]
    embedding: np.ndarray | None


@dataclass
class Observation:
    detection: Detection
    mouth_visible: bool
    mouth_activity: float


@dataclass
class Track:
    id: str
    created_frame: int
    first_timestamp: float
    last_timestamp: float
    lifecycle: Lifecycle = Lifecycle.TENTATIVE
    hits: list[int] = field(default_factory=list)
    observations: list[Observation] = field(default_factory=list)
    embedding: np.ndarray | None = None
    last_mouth_ratio: float | None = None
    confirmation_detections: int = 3
    confirmation_window_frames: int = 5

    def __post_init__(self) -> None:
        self.filter = cv2.KalmanFilter(8, 4)
        self.filter.transitionMatrix = np.eye(8, dtype=np.float32)
        self.filter.measurementMatrix = np.zeros((4, 8), dtype=np.float32)
        self.filter.measurementMatrix[:4, :4] = np.eye(4, dtype=np.float32)
        self.filter.processNoiseCov = np.eye(8, dtype=np.float32) * 1e-2
        self.filter.measurementNoiseCov = np.eye(4, dtype=np.float32) * 1e-1
        self.filter.errorCovPost = np.eye(8, dtype=np.float32)

    def initialize(self, detection: Detection) -> None:
        self.filter.statePost = np.zeros((8, 1), dtype=np.float32)
        self.filter.statePost[:4, 0] = box_measurement(detection.box)
        self.update(detection)

    def predict(self, timestamp: float) -> tuple[float, float, float, float]:
        elapsed = max(timestamp - self.last_timestamp, 1e-3)
        for index in range(4):
            self.filter.transitionMatrix[index, index + 4] = elapsed
        return measurement_box(self.filter.predict()[:4, 0])

    def update(self, detection: Detection) -> None:
        self.filter.correct(box_measurement(detection.box).reshape(4, 1))
        self.last_timestamp = detection.timestamp
        if self.lifecycle == Lifecycle.LOST:
            self.lifecycle = Lifecycle.CONFIRMED
        self.hits.append(detection.frame_index)
        self.hits = [
            value for value in self.hits
            if detection.frame_index-value < self.confirmation_window_frames
        ]
        if len(self.hits) >= self.confirmation_detections:
            self.lifecycle = Lifecycle.CONFIRMED
        mouth_visible, ratio = mouth_measurement(detection)
        activity = 0.0
        if mouth_visible and self.last_mouth_ratio is not None:
            activity = min(abs(ratio-self.last_mouth_ratio) * 6.0, 1.0)
        if mouth_visible:
            self.last_mouth_ratio = ratio
        self.observations.append(Observation(detection, mouth_visible, round(activity, 4)))
        if detection.embedding is not None:
            if self.embedding is None:
                self.embedding = detection.embedding.copy()
            else:
                self.embedding = normalize(self.embedding * 0.8 + detection.embedding * 0.2)


@dataclass(frozen=True)
class TrackerConfig:
    recording_id: str
    processing_version: int
    model_id: str
    high_confidence: float = 0.8
    low_confidence: float = 0.35
    iou_threshold: float = 0.2
    appearance_threshold: float = 0.35
    lost_timeout: float = 1.0
    reidentification_window: float = 10.0
    confirmation_detections: int = 3
    confirmation_window_frames: int = 5


class DenseFaceTracker:
    """Kalman + two-pass ByteTrack-style association with SFace re-identification."""

    def __init__(self, config: TrackerConfig) -> None:
        self.config = config
        self.tracks: list[Track] = []
        self._sequence = 0

    def update(self, frame_index: int, timestamp: float, detections: list[Detection]) -> None:
        candidates = [track for track in self.tracks if track.lifecycle != Lifecycle.ENDED]
        predictions = {track.id: track.predict(timestamp) for track in candidates}
        high = [item for item in detections if item.score >= self.config.high_confidence]
        low = [item for item in detections if self.config.low_confidence <= item.score < self.config.high_confidence]

        unmatched_after_high, unmatched_high = self._associate(candidates, high, predictions, appearance=True)
        second_pass = [track for track in unmatched_after_high if track.lifecycle in (Lifecycle.CONFIRMED, Lifecycle.LOST)]
        unmatched_after_low, _ = self._associate(second_pass, low, predictions, appearance=False)
        second_pass_ids = {track.id for track in second_pass}
        unmatched_tracks = [track for track in unmatched_after_high if track.id not in second_pass_ids]
        unmatched_tracks.extend(unmatched_after_low)
        unmatched_ids = {track.id for track in unmatched_tracks}
        matched_ids = {track.id for track in candidates if track.id not in unmatched_ids}

        for track in candidates:
            if track.id in matched_ids:
                continue
            missing = timestamp - track.last_timestamp
            if missing > self.config.reidentification_window:
                track.lifecycle = Lifecycle.ENDED
            elif missing > self.config.lost_timeout and track.lifecycle != Lifecycle.TENTATIVE:
                track.lifecycle = Lifecycle.LOST
            elif track.lifecycle == Lifecycle.TENTATIVE and frame_index-track.created_frame >= 5:
                track.lifecycle = Lifecycle.ENDED

        for detection in unmatched_high:
            self._sequence += 1
            identity = stable_track_id(self.config, detection, self._sequence)
            track = Track(
                identity, frame_index, timestamp, timestamp,
                confirmation_detections=self.config.confirmation_detections,
                confirmation_window_frames=self.config.confirmation_window_frames,
            )
            track.initialize(detection)
            self.tracks.append(track)

    def finish(self) -> list[Track]:
        for track in self.tracks:
            if track.lifecycle in (Lifecycle.CONFIRMED, Lifecycle.LOST):
                track.lifecycle = Lifecycle.ENDED
        return [
            track for track in self.tracks
            if len(track.observations) >= self.config.confirmation_detections
        ]

    def _associate(
        self,
        tracks: list[Track],
        detections: list[Detection],
        predictions: dict[str, tuple[float, float, float, float]],
        *,
        appearance: bool,
    ) -> tuple[list[Track], list[Detection]]:
        if not tracks or not detections:
            return list(tracks), list(detections)
        cost = np.full((len(tracks), len(detections)), 1e6, dtype=np.float64)
        for row, track in enumerate(tracks):
            for column, detection in enumerate(detections):
                overlap = box_iou(predictions[track.id], detection.box)
                similarity = cosine(track.embedding, detection.embedding)
                valid_motion = overlap >= self.config.iou_threshold
                valid_appearance = appearance and similarity >= self.config.appearance_threshold
                if valid_motion or valid_appearance:
                    appearance_cost = (1.0-similarity) * 0.45 if similarity >= -1 else 0.45
                    cost[row, column] = (1.0-overlap) * 0.55 + appearance_cost
        rows, columns = linear_sum_assignment(cost)
        matched_tracks: set[int] = set()
        matched_detections: set[int] = set()
        for row, column in zip(rows.tolist(), columns.tolist(), strict=True):
            if cost[row, column] >= 1e6:
                continue
            tracks[row].update(detections[column])
            matched_tracks.add(row)
            matched_detections.add(column)
        return (
            [track for index, track in enumerate(tracks) if index not in matched_tracks],
            [item for index, item in enumerate(detections) if index not in matched_detections],
        )


def stable_track_id(config: TrackerConfig, detection: Detection, sequence: int) -> str:
    center_x = detection.box[0] + detection.box[2] / 2
    center_y = detection.box[1] + detection.box[3] / 2
    source = "\0".join((
        config.recording_id,
        str(config.processing_version),
        config.model_id,
        f"{detection.timestamp:.6f}",
        f"{center_x:.1f}",
        f"{center_y:.1f}",
        str(sequence),
    ))
    return "person-track-" + hashlib.sha256(source.encode()).hexdigest()[:32]


def box_measurement(box: tuple[float, float, float, float]) -> np.ndarray:
    x, y, width, height = box
    return np.asarray((x + width/2, y + height/2, max(width, 1), max(height, 1)), dtype=np.float32)


def measurement_box(measurement: np.ndarray) -> tuple[float, float, float, float]:
    center_x, center_y, width, height = (float(value) for value in measurement)
    width, height = max(width, 1), max(height, 1)
    return center_x-width/2, center_y-height/2, width, height


def box_iou(left: tuple[float, float, float, float], right: tuple[float, float, float, float]) -> float:
    left_x, left_y, left_width, left_height = left
    right_x, right_y, right_width, right_height = right
    intersection_width = max(0.0, min(left_x+left_width, right_x+right_width)-max(left_x, right_x))
    intersection_height = max(0.0, min(left_y+left_height, right_y+right_height)-max(left_y, right_y))
    intersection = intersection_width * intersection_height
    union = left_width*left_height + right_width*right_height - intersection
    return intersection/union if union > 0 else 0.0


def cosine(left: np.ndarray | None, right: np.ndarray | None) -> float:
    if left is None or right is None or left.size != right.size:
        return -1.0
    return float(np.clip(np.dot(left, right), -1, 1))


def normalize(vector: np.ndarray) -> np.ndarray:
    norm = float(np.linalg.norm(vector))
    return vector/norm if math.isfinite(norm) and norm > 0 else vector


def mouth_measurement(detection: Detection) -> tuple[bool, float]:
    if len(detection.landmarks) != 5 or detection.box[2] < 1:
        return False, 0.0
    left, right = detection.landmarks[3], detection.landmarks[4]
    visible = all(math.isfinite(value) for point in (left, right) for value in point)
    ratio = abs(right[0]-left[0])/detection.box[2] if visible else 0.0
    return visible and ratio > 0.02, ratio
