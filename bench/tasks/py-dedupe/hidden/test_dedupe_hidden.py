import re
import unittest

import handlers


class TestHiddenDedupe(unittest.TestCase):
    def test_is_valid_email(self):
        good = ["a@b.co", "first.last@sub.example.org", "x+y@z.io"]
        bad = ["", "a@b", "a b@c.com", "@b.com", "a@@b.com", "a@b.c", "a@b.c0m"]
        for e in good:
            self.assertTrue(handlers.is_valid_email(e), e)
        for e in bad:
            self.assertFalse(handlers.is_valid_email(e), e)

    def test_single_regex(self):
        src = open(handlers.__file__).read()
        self.assertEqual(src.count("[^@\\s]+@[^@\\s]+"), 1, "the email pattern should appear once")

    def test_behaviour_unchanged(self):
        self.assertEqual(handlers.register({"email": " Ann@Example.com "}), {"ok": True, "user": "ann@example.com"})
        self.assertEqual(handlers.register({"email": "a@b"}), {"ok": False, "error": "invalid email"})
        self.assertEqual(handlers.invite({"email": " "}), {"ok": False, "error": "invalid email"})
        self.assertEqual(handlers.update_email("u", {"email": "C@D.EF"}), {"ok": True, "user": "u", "email": "c@d.ef"})


if __name__ == "__main__":
    unittest.main()
