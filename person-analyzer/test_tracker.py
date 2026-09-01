import unittest

import numpy as np

from app.main import Analyzer
from app.tracker import DenseFaceTracker, Detection, TrackerConfig


def detection(frame: int, x: float, vector: tuple[float, float], score: float = 0.9) -> Detection:
    return Detection(
        timestamp=frame/8,
        frame_index=frame,
        box=(x, 10, 80, 80),
        landmarks=((x+20, 30), (x+60, 30), (x+40, 45), (x+25, 65), (x+55, 65)),
        score=score,
        quality=0.9,
        quality_reasons=(),
        pose=(0, 0, 0, "frontal"),
        embedding=np.asarray(vector, dtype=np.float64),
    )


class TrackerTest(unittest.TestCase):
    def test_two_people_remain_independent_when_paths_cross(self) -> None:
        tracker = DenseFaceTracker(TrackerConfig("recording", 3, "sface"))
        for frame in range(8):
            tracker.update(frame, frame/8, [
                detection(frame, 10+frame*8, (1, 0)),
                detection(frame, 130-frame*8, (0, 1)),
            ])
        tracks = tracker.finish()
        self.assertEqual(2, len(tracks))
        self.assertEqual([8, 8], sorted(len(track.observations) for track in tracks))
        self.assertTrue(all(track.id.startswith("person-track-") for track in tracks))

    def test_empty_frame_does_not_reset_other_tracks(self) -> None:
        tracker = DenseFaceTracker(TrackerConfig("recording", 3, "sface"))
        for frame in range(3):
            tracker.update(frame, frame/8, [detection(frame, 10+frame, (1, 0))])
        tracker.update(3, 3/8, [])
        tracker.update(4, 4/8, [detection(4, 14, (1, 0))])
        tracks = tracker.finish()
        self.assertEqual(1, len(tracks))
        self.assertEqual(4, len(tracks[0].observations))

    def test_tentative_false_positive_is_not_returned(self) -> None:
        tracker = DenseFaceTracker(TrackerConfig("recording", 3, "sface"))
        tracker.update(0, 0, [detection(0, 10, (1, 0))])
        for frame in range(1, 7):
            tracker.update(frame, frame/8, [])
        self.assertEqual([], tracker.finish())

    def test_confirmed_track_serializes_detection_confidence(self) -> None:
        tracker = DenseFaceTracker(TrackerConfig("recording", 3, "sface"))
        for frame, score in enumerate((0.92, 0.81, 0.87)):
            tracker.update(frame, frame/8, [detection(frame, 10+frame, (1, 0), score)])

        result = Analyzer._serialize_track(tracker.finish()[0], 5)

        self.assertEqual(0.81, result.tracking_confidence)


if __name__ == "__main__":
    unittest.main()
