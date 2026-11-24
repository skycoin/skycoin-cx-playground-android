#!/bin/sh
# tool-test.sh AGENT TEST_DIR runs a generated test, before and after plus
# replay determinism in one:
#
# 1. Two replays of the port from TEST_DIR/tape.txt print byte identical
#    output, equal to TEST_DIR/expected-output.txt.
# 2. The unmodified original, fed the inbound messages and clock reads of the
#    tape and started on a copy of TEST_DIR/spiffs, prints the same bytes,
#    and every model request it makes is found in a replay store filled from
#    the net-request and net-reply records of the tape.
set -eu
agent=$1
here=$(cd "$2" && pwd)
port_dir=$(cd "$here/../../port" && pwd)
before_var=$(echo "$agent" | tr a-z A-Z)_BEFORE
eval "before_bin=\${$before_var:-}"
: "${CX:?set CX}" "${BACKENDSTORE:?set BACKENDSTORE}" "${before_bin:?set $before_var}"
run_dir=$(mktemp -d)
cleanup() { [ -n "$run_dir" ] && rm -rf "$run_dir"; }
trap cleanup EXIT
cp "$here/tape.txt" "$run_dir/"
mkdir "$run_dir/before"
[ -d "$here/spiffs" ] && cp -r "$here/spiffs" "$run_dir/before/spiffs"
cd "$run_dir"

for replay in first second; do
	"$CX" --enable=file,net,process "$port_dir"/*.cx ++mode=replay ++tape=tape.txt > "$replay.txt" 2> "$replay-stderr.txt" || {
		cat "$replay.txt"; echo "$replay replay of the port failed" >&2; exit 1; }
done
cmp first.txt second.txt
cmp first.txt "$here/expected-output.txt"

"$BACKENDSTORE" import -tape tape.txt -store store.bolt
"$BACKENDSTORE" serve -store store.bolt -mode replay -address-file store.url 2> store-stderr.txt &
store_pid=$!
while [ ! -s store.url ]; do sleep 0.1; done
before_status=0
(cd before && "$before_bin" ../tape.txt "$(cat ../store.url)" "$BACKENDSTORE") > before.txt 2> before-stderr.txt || before_status=$?
"$BACKENDSTORE" stop -url "$(cat store.url)"
store_status=0
wait "$store_pid" || store_status=$?
cat store-stderr.txt
[ "$before_status" -eq 0 ] || { cat before.txt; echo "original exited with $before_status" >&2; exit 1; }
[ "$store_status" -eq 0 ] || { echo "the store missed requests of the original" >&2; exit 1; }
cmp before.txt first.txt
echo "before and after: $(wc -c < first.txt) identical bytes, $(grep -c '^net-request	' tape.txt) requests of the original found in the store; two replays identical"
