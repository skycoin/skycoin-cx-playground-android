#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
expect_output "6" src/a.cx src/b.cx src/c.cx
echo solved
