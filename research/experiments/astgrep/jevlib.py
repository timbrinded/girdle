"""Shared Jev access for the fan-out experiments. The key is read from the
interactive shell profile and never printed."""
import json, pathlib, subprocess, time, urllib.request, urllib.error

# The repository root: these scripts live in research/experiments/fanout.
REPO = str(pathlib.Path(__file__).resolve().parents[3])

_KEY = None
def key():
    global _KEY
    if _KEY is None:
        _KEY = subprocess.run(['zsh', '-ic', 'printf %s "$TYPESAFE_API_KEY"'], capture_output=True, text=True).stdout
    return _KEY

def noul(q): return {"type": "noul", "instructions": q}
def choice(q, opts): return {"type": "choice", "instructions": q, "criteria": opts}
def score(q, levels): return {"type": "score", "instructions": q, "criteria": levels}

def ask(state, questions, retries=4, timeout=120):
    body = json.dumps({"model": "jev-1.13.0", "state": state, "questions": questions}).encode()
    for attempt in range(retries):
        req = urllib.request.Request("https://api.typesafe.ai/v1/systemone", data=body,
            headers={"Content-Type": "application/json", "Authorization": "Bearer " + key()})
        t = time.time()
        try:
            r = json.load(urllib.request.urlopen(req, timeout=timeout))
            r["_ms"] = int((time.time() - t) * 1000)
            return r
        except urllib.error.HTTPError as e:
            msg = e.read().decode(errors="replace")[:300]
            if e.code in (429, 500, 502, 503, 504) and attempt < retries - 1:
                time.sleep(2 ** attempt); continue
            raise RuntimeError(f"HTTP {e.code}: {msg}")
        except Exception:
            if attempt < retries - 1:
                time.sleep(2 ** attempt); continue
            raise
