set -e
[ "$(grep -c "test(" slug.test.js)" -gt 1 ] || ls *.test.js | grep -v "^slug.test.js$" >/dev/null || { echo "FAIL: no tests added"; exit 1; }
cp "$TASK_DIR/hidden/slug.hidden.test.js" .
node --test
