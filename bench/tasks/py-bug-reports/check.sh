set -e
n=$(cat test_*.py | grep -c "def test")
[ "$n" -ge 6 ] || { echo "FAIL: expected tests for each bug report, found $n tests"; exit 1; }
cp "$TASK_DIR/hidden/test_invoice_hidden.py" .
python3 -m unittest -q
