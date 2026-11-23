#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
! grep -q "func greeting" main.cx || fail "main.cx still defines greeting"
grep -q "func greeting" util.cx || fail "util.cx does not define greeting"
expect_output "Welcome, Ana" main.cx util.cx
echo solved
