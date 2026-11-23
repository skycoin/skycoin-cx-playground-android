#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
! grep -q gCount state.cx main.cx || fail "gCount is still used"
grep -q gTotal state.cx || fail "state.cx does not declare gTotal"
expect_output "20" main.cx state.cx
echo solved
