#!/bin/sh
. "$(dirname "$0")/../../check-lib.sh"
[ -f NOTES.md ] || fail "NOTES.md is missing"
[ "$(cat NOTES.md)" = "Build with cx main.cx" ] || fail "NOTES.md holds '$(cat NOTES.md)'"
echo solved
