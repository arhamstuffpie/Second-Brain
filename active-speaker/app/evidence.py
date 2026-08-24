from __future__ import annotations

import math
from collections import defaultdict
from typing import Any


def build_evidence(
    frames: list[dict[str, Any]], metadata: dict[str, Any], fps: float
) -> tuple[list[dict[str, Any]], str]:
    """Map TalkNet's face tracks to app tracks when that mapping is unambiguous."""
    if fps <= 0:
        raise ValueError("video frame rate must be positive")

    tracks = metadata.get("person_tracks", [])
    grouped: dict[str, dict[str, Any]] = defaultdict(
        lambda: {"segment_ids": [], "scores": [], "coverages": [], "frame_ids": []}
    )
    ambiguous_segments = 0

    for segment in metadata.get("segments", []):
        segment_id = str(segment.get("id", "")).strip()
        start = float(segment.get("start_time", 0))
        end = float(segment.get("end_time", 0))
        if not segment_id or end <= start:
            continue

        candidates = [track for track in tracks if overlaps(track, start, end)]
        if len(candidates) != 1:
            if candidates:
                ambiguous_segments += 1
            continue

        segment_frames = [
            frame for frame in frames
            if start <= float(frame.get("frame_number", -1)) / fps <= end
        ]
        if not segment_frames:
            continue

        active_by_track: dict[str, list[float]] = defaultdict(list)
        visible_by_track: dict[str, int] = defaultdict(int)
        for frame in segment_frames:
            seen: set[str] = set()
            for face in frame.get("faces", []):
                track_id = str(face.get("track_id", ""))
                if not track_id:
                    continue
                if track_id not in seen:
                    visible_by_track[track_id] += 1
                    seen.add(track_id)
                if face.get("speaking"):
                    active_by_track[track_id].append(sigmoid(float(face.get("raw_score", 0))))

        active_tracks = [track_id for track_id, scores in active_by_track.items() if scores]
        if len(active_tracks) != 1:
            if len(active_tracks) > 1:
                ambiguous_segments += 1
            continue

        talknet_track = active_tracks[0]
        app_track = candidates[0]
        app_track_id = str(app_track.get("id", "")).strip()
        if not app_track_id:
            continue

        item = grouped[app_track_id]
        item["segment_ids"].append(segment_id)
        item["scores"].append(sum(active_by_track[talknet_track]) / len(active_by_track[talknet_track]))
        item["coverages"].append(visible_by_track[talknet_track] / len(segment_frames))
        for frame_id in app_track.get("evidence_frame_ids", []):
            if frame_id not in item["frame_ids"]:
                item["frame_ids"].append(frame_id)

    evidence = [
        {
            "person_track_id": track_id,
            "segment_ids": item["segment_ids"],
            "score": clamp(sum(item["scores"]) / len(item["scores"])),
            "visible_mouth_coverage": clamp(sum(item["coverages"]) / len(item["coverages"])),
            "overlapping_conflict": False,
            "evidence_frame_ids": item["frame_ids"],
        }
        for track_id, item in grouped.items()
        if item["segment_ids"]
    ]
    warning = ""
    if ambiguous_segments:
        warning = f"skipped {ambiguous_segments} segment(s) with ambiguous visual speakers"
    return evidence, warning


def overlaps(track: dict[str, Any], start: float, end: float) -> bool:
    return float(track.get("start_time", 0)) < end and float(track.get("end_time", 0)) > start


def sigmoid(value: float) -> float:
    if value >= 0:
        return 1 / (1 + math.exp(-value))
    exponential = math.exp(value)
    return exponential / (1 + exponential)


def clamp(value: float) -> float:
    return max(0.0, min(1.0, value))
