#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
expect_output "Hello, world" greet.cx
echo solved
