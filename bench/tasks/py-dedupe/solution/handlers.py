import re

_EMAIL = re.compile(r"[^@\s]+@[^@\s]+\.[A-Za-z]{2,}")


def is_valid_email(email: str) -> bool:
    return bool(email) and _EMAIL.fullmatch(email) is not None


def register(form):
    email = form.get("email", "").strip()
    if not is_valid_email(email):
        return {"ok": False, "error": "invalid email"}
    return {"ok": True, "user": email.lower()}


def invite(form):
    email = form.get("email", "").strip()
    if not is_valid_email(email):
        return {"ok": False, "error": "invalid email"}
    return {"ok": True, "invited": email.lower()}


def update_email(user, form):
    email = form.get("email", "").strip()
    if not is_valid_email(email):
        return {"ok": False, "error": "invalid email"}
    return {"ok": True, "user": user, "email": email.lower()}
