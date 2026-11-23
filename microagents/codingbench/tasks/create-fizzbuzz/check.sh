#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
expect_output "$(printf '1\n2\nFizz\n4\nBuzz\nFizz\n7\n8\nFizz\nBuzz\n11\nFizz\n13\n14\nFizzBuzz')" fizzbuzz.cx
echo solved
