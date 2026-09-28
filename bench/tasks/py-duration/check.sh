set -e
cmp -s "$TASK_DIR/repo/test_durations.py" test_durations.py || grep -q "parse_duration" test_durations.py
[ "$(grep -c "def test" test_durations.py)" -gt 2 ] || ls test_*.py | grep -v test_durations.py >/dev/null || { echo "FAIL: no tests added"; exit 1; }
cp "$TASK_DIR/hidden/test_durations_hidden.py" .
python3 -m unittest -q
