import unittest

from app.timeline import Turn, build_regions


class TimelineTest(unittest.TestCase):
    def test_preserves_overlap_and_silence_boundaries(self) -> None:
        regions = build_regions("r1", 3, [Turn(1, 4, "A"), Turn(2, 3, "B")], 5)
        self.assertEqual(
            [
                (0, 1, (), "silence"),
                (1, 2, ("A",), "speech"),
                (2, 3, ("A", "B"), "overlap"),
                (3, 4, ("A",), "speech"),
                (4, 5, (), "silence"),
            ],
            [(item.start, item.end, item.speakers, item.kind) for item in regions],
        )

    def test_ids_are_deterministic_and_versioned(self) -> None:
        turns = [Turn(0, 1, "A")]
        first = build_regions("r1", 3, turns, 1)[0].id
        self.assertEqual(first, build_regions("r1", 3, turns, 1)[0].id)
        self.assertNotEqual(first, build_regions("r1", 4, turns, 1)[0].id)


if __name__ == "__main__":
    unittest.main()
