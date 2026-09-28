import textutils


def display_name(raw):
    return textutils.title_case(textutils.strip_accents(raw.strip()))
