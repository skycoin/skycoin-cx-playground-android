#!/bin/sh
# R14 and R16: two replays of the port from tape.txt print byte identical
# output, equal to expected-output.txt.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
port_dir=$(cd "$here/../../port" && pwd)
: "${CX:?set CX}"
run_dir=$(mktemp -d)
cleanup() { [ -n "$run_dir" ] && rm -rf "$run_dir"; }
trap cleanup EXIT
cp "$here/tape.txt" "$run_dir/"
cd "$run_dir"
for replay in first second; do
	"$CX" --enable=file,net,process "$port_dir"/*.cx ++mode=replay ++tape=tape.txt > "$replay.txt" 2> "$replay-stderr.txt" || {
		cat "$replay.txt"; echo "$replay replay exited with $?" >&2; exit 1; }
done
cmp first.txt second.txt
cmp first.txt "$here/expected-output.txt"
echo "two replays: $(wc -c < first.txt) identical bytes, equal to expected-output.txt"
