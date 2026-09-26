import unittest

from durations import parse_duration


class TestHiddenParseDuration(unittest.TestCase):
    def test_valid(self):
        cases = {
            "1h30m": 5400, "45s": 45, "2h": 7200, "1h 5m 3s": 3903, "90m": 5400,
            "30": 30, "1H30M": 5400, "  2m  ": 120, "0s": 0, "10h 0m": 36000, "1m1s": 61,
        }
        for text, want in cases.items():
            with self.subTest(text=text):
                self.assertEqual(parse_duration(text), want)

    def test_invalid(self):
        for text in ["", "   ", "1x", "-5s", "1.5h", "1m1m", "5s1m", "1h30", "h", "1h -2m", "abc"]:
            with self.subTest(text=text):
                with self.assertRaises(ValueError):
                    parse_duration(text)


if __name__ == "__main__":
    unittest.main()
