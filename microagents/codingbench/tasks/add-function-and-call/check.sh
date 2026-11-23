#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
grep -q "func square" lib.cx || fail "lib.cx has no square"
expect_output "$(printf '8\n49')" main.cx lib.cx
echo solved
