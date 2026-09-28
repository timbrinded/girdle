set -e
# SWE-bench style: the agent's own test changes are set aside, and the
# upstream tests from the fix commit decide.
git checkout -q HEAD -- tests 2>/dev/null || true
git ls-files -z --others --exclude-standard -- tests | xargs -0 rm -f
git apply "$TASK_DIR/hidden/tests.patch"
python3 -m unittest discover -s tests -t .
