from textutils import normalize_space, truncate, word_count


def summarize(text, width=40):
    clean = normalize_space(text)
    return f"{truncate(clean, width)} ({word_count(clean)} words)"
