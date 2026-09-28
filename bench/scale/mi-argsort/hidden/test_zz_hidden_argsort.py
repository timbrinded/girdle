import random
from pathlib import Path
from unittest import TestCase

import more_itertools
import more_itertools as mi


class ZzHiddenArgsort(TestCase):
    def test_examples(self):
        self.assertEqual(mi.argsort([30, 10, 20]), [1, 2, 0])
        self.assertEqual(mi.argsort([]), [])
        self.assertEqual(mi.argsort('cab'), [1, 2, 0])
        self.assertEqual(mi.argsort(iter([3, 1, 2])), [1, 2, 0])

    def test_matches_sorted(self):
        rng = random.Random(7)
        for _ in range(200):
            items = [rng.randint(0, 5) for _ in range(rng.randint(0, 12))]
            for reverse in (False, True):
                want = sorted(range(len(items)), key=items.__getitem__, reverse=reverse)
                self.assertEqual(mi.argsort(items, reverse=reverse), want)
                self.assertEqual(mi.argsort(iter(items), reverse=reverse), want)

    def test_key(self):
        self.assertEqual(mi.argsort(['b', 'A', 'c'], key=str.lower), [1, 0, 2])
        self.assertEqual(mi.argsort([-3, 1, -2], key=abs, reverse=True), [0, 2, 1])

    def test_keyword_only(self):
        with self.assertRaises(TypeError):
            mi.argsort([1, 2], str)

    def test_exported_stubbed_documented_tested(self):
        self.assertIn('argsort', more_itertools.__all__ if hasattr(more_itertools, '__all__') else dir(more_itertools))
        from more_itertools import argsort  # noqa: F401
        root = Path(__file__).resolve().parent.parent
        self.assertIn('def argsort(', (root / 'more_itertools' / 'more.pyi').read_text())
        self.assertIn('argsort', (root / 'docs' / 'api.rst').read_text())
        self.assertIn('argsort', (root / 'tests' / 'test_more.py').read_text())
