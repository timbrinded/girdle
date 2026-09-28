"""Small text helpers."""

import re
import unicodedata


def normalize_space(s):
    return " ".join(s.split())


def strip_accents(s):
    return "".join(c for c in unicodedata.normalize("NFD", s) if unicodedata.category(c) != "Mn")


def word_count(s):
    return len(re.findall(r"\w+", s))


def truncate(s, n):
    return s if len(s) <= n else s[: n - 1] + "…"


def title_case(s):
    return " ".join(w.capitalize() for w in s.split())


def is_palindrome(s):
    t = re.sub(r"\W", "", s.lower())
    return t == t[::-1]


def count_vowels(s):
    return sum(c in "aeiou" for c in s.lower())


def reverse_words(s):
    return " ".join(reversed(s.split()))
