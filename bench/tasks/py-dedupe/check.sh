set -e
grep -l "is_valid_email" test_*.py >/dev/null || { echo "FAIL: no tests for is_valid_email"; exit 1; }
cp "$TASK_DIR/hidden/test_dedupe_hidden.py" .
python3 -m unittest -q
