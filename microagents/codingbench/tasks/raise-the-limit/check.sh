#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
expect_output "$(printf '1\n2\n3\n4\n5')" count.cx config.cx
grep -q "Limit i32 = 5" config.cx || fail "config.cx does not set Limit to 5"
echo solved
