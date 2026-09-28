set -e
cp "$TASK_DIR/hidden/test_zz_hidden_sliding_window.py" tests/
python3 -m unittest discover -s tests -t .
