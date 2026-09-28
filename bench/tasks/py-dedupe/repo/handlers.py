import re


def register(form):
    email = form.get("email", "").strip()
    if not email or not re.fullmatch(r"[^@\s]+@[^@\s]+\.[A-Za-z]{2,}", email):
        return {"ok": False, "error": "invalid email"}
    return {"ok": True, "user": email.lower()}


def invite(form):
    email = form.get("email", "").strip()
    if not email or not re.fullmatch(r"[^@\s]+@[^@\s]+\.[A-Za-z]{2,}", email):
        return {"ok": False, "error": "invalid email"}
    return {"ok": True, "invited": email.lower()}


def update_email(user, form):
    email = form.get("email", "").strip()
    if not email or not re.fullmatch(r"[^@\s]+@[^@\s]+\.[A-Za-z]{2,}", email):
        return {"ok": False, "error": "invalid email"}
    return {"ok": True, "user": user, "email": email.lower()}
