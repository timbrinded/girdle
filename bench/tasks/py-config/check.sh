set -e
grep -q "APP_PORT" README.md && grep -q "APP_DEBUG" README.md || { echo "FAIL: README does not document the env vars"; exit 1; }
grep -qi "65535" README.md || { echo "FAIL: README does not document validation"; exit 1; }
grep -l "APP_" test_*.py >/dev/null || { echo "FAIL: no tests for env overrides"; exit 1; }
grep -l "validate" test_*.py >/dev/null || { echo "FAIL: no tests for validate"; exit 1; }
cp "$TASK_DIR/hidden/test_config_hidden.py" .
python3 -m unittest -q
