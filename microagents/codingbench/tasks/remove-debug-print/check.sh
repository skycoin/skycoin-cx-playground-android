#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
expect_output "Hi Ana" main.cx
echo solved
