#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
expect_output "15" sum.cx
echo solved
