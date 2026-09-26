set -e
[ "$(cat *.test.js | grep -c "test(")" -gt 1 ] || { echo "FAIL: no tests added"; exit 1; }
cp "$TASK_DIR/hidden/csv.hidden.test.js" .
node --test
