from __future__ import annotations

import hashlib
from dataclasses import dataclass


@dataclass(frozen=True)
class Turn:
    start: float
    end: float
    speaker: str


@dataclass(frozen=True)
class Region:
    id: str
    start: float
    end: float
    speakers: tuple[str, ...]

    @property
    def kind(self) -> str:
        if not self.speakers:
            return "silence"
        return "overlap" if len(self.speakers) > 1 else "speech"


def build_regions(recording_id: str, version: int, turns: list[Turn], duration: float) -> list[Region]:
    boundaries = {0.0, max(duration, 0.0)}
    valid: list[Turn] = []
    for turn in turns:
        start, end = max(0.0, turn.start), min(duration, turn.end)
        if end <= start or not turn.speaker:
            continue
        valid.append(Turn(start, end, turn.speaker))
        boundaries.update((start, end))
    points = sorted(boundaries)
    atomic: list[tuple[float, float, tuple[str, ...]]] = []
    for start, end in zip(points, points[1:]):
        speakers = tuple(sorted({turn.speaker for turn in valid if turn.start < end and turn.end > start}))
        if end > start:
            atomic.append((start, end, speakers))
    merged: list[tuple[float, float, tuple[str, ...]]] = []
    for start, end, speakers in atomic:
        if merged and merged[-1][2] == speakers:
            merged[-1] = (merged[-1][0], end, speakers)
        else:
            merged.append((start, end, speakers))
    return [Region(stable_region_id(recording_id, version, start, end, speakers), start, end, speakers) for start, end, speakers in merged]


def stable_region_id(recording_id: str, version: int, start: float, end: float, speakers: tuple[str, ...]) -> str:
    source = "\0".join((recording_id, str(version), f"{start:.6f}", f"{end:.6f}", *speakers))
    return "audio-region-" + hashlib.sha256(source.encode()).hexdigest()[:32]


def stable_source_id(region_id: str, source_index: int) -> str:
    return "audio-source-" + hashlib.sha256(f"{region_id}\0{source_index}".encode()).hexdigest()[:32]
