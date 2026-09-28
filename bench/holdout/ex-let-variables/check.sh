set -e
# SWE-bench style: the agent's own test changes are set aside, and the
# upstream tests from the fix commit decide. The vm package's tests are left
# out: the fix commit also removed Program.FuncNames, a refactor unrelated to
# the task, which the base vm tests still use.
git ls-files -z -- '*_test.go' '*/testdata/*' 'testdata/*' '_test/*' '*/_test/*' | xargs -0 git checkout -q HEAD -- 2>/dev/null || true
git ls-files -z --others --exclude-standard -- '*_test.go' | xargs -0 rm -f
git apply "$TASK_DIR/hidden/tests.patch"
go test -vet=off ./. ./ast ./checker ./compiler ./parser ./parser/lexer
