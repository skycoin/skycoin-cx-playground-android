#!/bin/sh
# R14: tape.txt is the replay-determinism tape with one byte of SOUL.md
# changed. The port builds a different model request from it, so the replay
# must stop with exit code 3 and the message in expected-output.txt.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
port_dir=$(cd "$here/../../port" && pwd)
: "${CX:?set CX}"
run_dir=$(mktemp -d)
cleanup() { [ -n "$run_dir" ] && rm -rf "$run_dir"; }
trap cleanup EXIT
cp "$here/tape.txt" "$run_dir/"
cd "$run_dir"
status=0
"$CX" "$port_dir"/*.cx ++mode=replay ++tape=tape.txt > output.txt 2> stderr.txt || status=$?
[ "$status" -eq 3 ] || { cat output.txt; echo "want exit code 3, got $status" >&2; exit 1; }
cmp output.txt "$here/expected-output.txt"
echo "the replay stopped at the diverging request: $(tail -1 output.txt)"
