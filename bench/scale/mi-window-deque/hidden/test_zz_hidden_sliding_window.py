from unittest import TestCase

import more_itertools as mi


class ZzHiddenSlidingWindow(TestCase):
    def test_every_width(self):
        for size in (0, 1, 5, 20, 21, 22, 30, 60):
            items = list(range(size))
            for n in range(1, 45):
                want = [tuple(items[i:i + n]) for i in range(len(items) - n + 1)]
                self.assertEqual(list(mi.sliding_window(items, n)), want, (size, n))
                self.assertEqual(list(mi.sliding_window(iter(items), n)), want, (size, n))
