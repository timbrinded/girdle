from collections import deque
from itertools import product
from unittest import TestCase

import more_itertools as mi


def zz_reference(iterable, n, key=None):
    recent = deque(maxlen=n)
    for item in iterable:
        k = key(item) if key else item
        if k not in list(recent)[-(n - 1):] if n > 1 else True:
            yield item
        recent.append(k)


class ZzHiddenUniqueInWindow(TestCase):
    def test_documented(self):
        self.assertEqual(list(mi.unique_in_window([0, 1, 0, 2, 3, 0], 3)), [0, 1, 2, 3, 0])
        self.assertEqual(list(mi.unique_in_window([1, 2, 3, 4, 1, 5, 1], 3)), [1, 2, 3, 4, 1, 5])
        self.assertEqual(list(mi.unique_in_window('aab', 1)), ['a', 'a', 'b'])

    def test_against_reference(self):
        for n in range(1, 5):
            for items in product('abc', repeat=6):
                self.assertEqual(
                    list(mi.unique_in_window(items, n)),
                    list(zz_reference(items, n)),
                    (items, n),
                )

    def test_key(self):
        self.assertEqual(
            list(mi.unique_in_window('abAcdaB', 3, key=str.lower)),
            ['a', 'b', 'c', 'd', 'a', 'B'],
        )
