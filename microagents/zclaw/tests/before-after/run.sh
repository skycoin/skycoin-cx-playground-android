#!/bin/sh
# R15 and R16: the unmodified zclaw agent ("before"), driven by the upstream
# host test mocks, and the CX port ("after") print the same output for the
# inbound messages, uptimes and clock reads of tape.txt. The original's model
# requests are answered by store.bolt in replay mode, which fails on any
# request that is not byte identical to one the port recorded.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
port_dir=$(cd "$here/../../port" && pwd)
: "${CX:?set CX}" "${BACKENDSTORE:?set BACKENDSTORE}" "${ZCLAW_BEFORE:?set ZCLAW_BEFORE}"
run_dir=$(mktemp -d)
cleanup() { [ -n "$run_dir" ] && rm -rf "$run_dir"; }
trap cleanup EXIT
cp "$here/tape.txt" "$here/store.bolt" "$run_dir/"
cd "$run_dir"

"$CX" "$port_dir"/*.cx ++mode=replay ++tape=tape.txt > after.txt 2> after-stderr.txt || {
	cat after.txt; echo "port replay failed" >&2; exit 1; }

mkdir before
"$BACKENDSTORE" serve -store store.bolt -mode replay -address-file store.url 2> store-stderr.txt &
store_pid=$!
while [ ! -s store.url ]; do sleep 0.1; done
before_status=0
(cd before && "$ZCLAW_BEFORE" ../tape.txt "$(cat ../store.url)" "$BACKENDSTORE") > before.txt 2> before-stderr.txt || before_status=$?
"$BACKENDSTORE" stop -url "$(cat store.url)"
store_status=0
wait "$store_pid" || store_status=$?
cat store-stderr.txt
[ "$before_status" -eq 0 ] || { cat before.txt; echo "original exited with $before_status" >&2; exit 1; }
[ "$store_status" -eq 0 ] || { echo "the store missed requests of the original" >&2; exit 1; }

cmp before.txt after.txt
cmp after.txt "$here/expected-output.txt"
echo "before and after: $(wc -c < after.txt) identical bytes, every request of the original found in the store"
