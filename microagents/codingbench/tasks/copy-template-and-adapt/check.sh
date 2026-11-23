#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
expect_output "app ready" app.cx
expect_output "TEMPLATE" template.cx
echo solved
