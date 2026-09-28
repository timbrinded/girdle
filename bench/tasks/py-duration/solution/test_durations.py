import unittest

from durations import parse_duration


class TestParseDuration(unittest.TestCase):
    def test_hours_and_minutes(self):
        self.assertEqual(parse_duration("1h30m"), 5400)

    def test_seconds(self):
        self.assertEqual(parse_duration("45s"), 45)

    def test_bare_number(self):
        self.assertEqual(parse_duration("30"), 30)

    def test_invalid(self):
        with self.assertRaises(ValueError):
            parse_duration("1h30")


if __name__ == "__main__":
    unittest.main()
