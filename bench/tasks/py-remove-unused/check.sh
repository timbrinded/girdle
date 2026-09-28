set -e
python3 - <<'PY'
import ast, sys
defs = {n.name for n in ast.parse(open("textutils.py").read()).body if isinstance(n, ast.FunctionDef)}
keep = {"normalize_space", "strip_accents", "word_count", "truncate", "title_case"}
gone = {"is_palindrome", "count_vowels", "reverse_words"}
missing = keep - defs
left = gone & defs
if missing or left:
    sys.exit(f"FAIL: removed used functions {sorted(missing)}; left unused {sorted(left)}")
PY
cmp -s "$TASK_DIR/repo/test_app.py" test_app.py || { echo "FAIL: tests changed"; exit 1; }
python3 -m unittest -q
