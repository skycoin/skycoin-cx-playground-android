#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
expect_output "hello" hello.cx
echo solved
