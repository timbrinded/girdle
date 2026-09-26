import unittest

from handlers import is_valid_email


class TestIsValidEmail(unittest.TestCase):
    def test_cases(self):
        self.assertTrue(is_valid_email("a@b.co"))
        self.assertFalse(is_valid_email("a@b"))


if __name__ == "__main__":
    unittest.main()
