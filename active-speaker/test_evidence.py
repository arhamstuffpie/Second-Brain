import math
import unittest

from app.evidence import build_evidence


class EvidenceTest(unittest.TestCase):
    def test_maps_one_visible_track_to_multiple_segments(self):
        metadata = {
            "person_tracks": [{
                "id": "person-track-1", "start_time": 0, "end_time": 3,
                "evidence_frame_ids": ["frame-1"],
            }],
            "segments": [
                {"id": "segment-1", "start_time": 0, "end_time": 1},
                {"id": "segment-2", "start_time": 1, "end_time": 2},
            ],
        }
        frames = [
            {"frame_number": frame, "faces": [{
                "track_id": 7, "raw_score": 2.0, "speaking": True,
            }]}
            for frame in range(51)
        ]

        evidence, warning = build_evidence(frames, metadata, 25)

        self.assertEqual(warning, "")
        self.assertEqual(evidence[0]["person_track_id"], "person-track-1")
        self.assertEqual(evidence[0]["segment_ids"], ["segment-1", "segment-2"])
        self.assertTrue(math.isclose(evidence[0]["score"], 0.880797, rel_tol=1e-5))
        self.assertEqual(evidence[0]["visible_mouth_coverage"], 1)

    def test_skips_segment_with_two_application_tracks(self):
        metadata = {
            "person_tracks": [
                {"id": "one", "start_time": 0, "end_time": 2},
                {"id": "two", "start_time": 0, "end_time": 2},
            ],
            "segments": [{"id": "segment", "start_time": 0, "end_time": 1}],
        }

        evidence, warning = build_evidence([], metadata, 25)

        self.assertEqual(evidence, [])
        self.assertIn("skipped 1 segment", warning)


if __name__ == "__main__":
    unittest.main()
