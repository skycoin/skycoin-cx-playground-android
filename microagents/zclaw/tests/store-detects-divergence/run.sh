#!/bin/sh
# R15: the original runs on the before-after store with a tape whose first
# inbound message has one byte changed. Its model requests then differ from
# the recorded ones, so the replay store must report misses and exit
# non-zero.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${BACKENDSTORE:?set BACKENDSTORE}" "${ZCLAW_BEFORE:?set ZCLAW_BEFORE}"
run_dir=$(mktemp -d)
cleanup() { [ -n "$run_dir" ] && rm -rf "$run_dir"; }
trap cleanup EXIT
mkdir "$run_dir/before"
cp "$here/tape.txt" "$here/store.bolt" "$run_dir/"
cd "$run_dir"
"$BACKENDSTORE" serve -store store.bolt -mode replay -address-file store.url 2> store-stderr.txt &
store_pid=$!
while [ ! -s store.url ]; do sleep 0.1; done
(cd before && "$ZCLAW_BEFORE" ../tape.txt "$(cat ../store.url)" "$BACKENDSTORE") > before.txt 2> before-stderr.txt || true
"$BACKENDSTORE" stop -url "$(cat store.url)"
store_status=0
wait "$store_pid" || store_status=$?
[ "$store_status" -ne 0 ] || { echo "want replay misses, the store answered every request" >&2; exit 1; }
misses=$(grep -c "replay miss" store-stderr.txt)
[ "$misses" -gt 0 ]
cmp before.txt "$here/expected-output.txt"
echo "the store refused $misses diverging requests and the original printed $(grep -c "after retries" before.txt) retry errors"
