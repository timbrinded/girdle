import unittest

from names import display_name
from report import summarize


class TestApp(unittest.TestCase):
    def test_summarize(self):
        self.assertEqual(summarize("  hello   world  "), "hello world (2 words)")

    def test_display_name(self):
        self.assertEqual(display_name(" josé garcía "), "Jose Garcia")


if __name__ == "__main__":
    unittest.main()
