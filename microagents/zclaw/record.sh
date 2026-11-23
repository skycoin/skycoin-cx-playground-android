#!/bin/sh
# record.sh TEST_DIR records a test: it runs the CX port of zclaw in record
# mode on TEST_DIR/inbound.txt and writes TEST_DIR/tape.txt,
# TEST_DIR/store.bolt and TEST_DIR/expected-output.txt.
#
# The model is the deterministic stub unless UPSTREAM is set, for example
# UPSTREAM=https://api.anthropic.com/v1/messages with ANTHROPIC_API_KEY set.
# CX and BACKENDSTORE name the binaries, see ../run-tests.sh.
set -eu
test_dir=$(cd "$1" && pwd)
port_dir=$(cd "$(dirname "$0")/port" && pwd)
: "${CX:?set CX to the cx binary}"
: "${BACKENDSTORE:?set BACKENDSTORE to the backendstore binary}"
[ -x "$CX" ] && [ -x "$BACKENDSTORE" ] || { echo "record: CX and BACKENDSTORE must be executable files" >&2; exit 1; }

run_dir=$(mktemp -d)
cleanup() { [ -n "$run_dir" ] && rm -rf "$run_dir"; }
trap cleanup EXIT
cp "$test_dir/inbound.txt" "$run_dir/inbound.txt"
cd "$run_dir"

upstream=${UPSTREAM:-}
if [ -z "$upstream" ]; then
	"$BACKENDSTORE" stub -address-file stub.url &
	while [ ! -s stub.url ]; do sleep 0.1; done
	upstream="$(cat stub.url)/v1/messages"
fi
"$BACKENDSTORE" serve -store store.bolt -mode record -listen 127.0.0.1:18080 -upstream "$upstream" -address-file store.url &
while [ ! -s store.url ]; do sleep 0.1; done

status=0
"$CX" "$port_dir"/*.cx ++mode=record ++tape=tape.txt ++inbound=inbound.txt \
	"++client=$BACKENDSTORE" "++backend=$(cat store.url)" > output.txt 2> cx-stderr.txt || status=$?
"$BACKENDSTORE" stop -url "$(cat store.url)"
[ -s stub.url ] && "$BACKENDSTORE" stop -url "$(cat stub.url)"
wait
if [ "$status" -ne 0 ]; then
	cat output.txt
	echo "record: the port exited with $status" >&2
	exit 1
fi
cp tape.txt store.bolt "$test_dir/"
cp output.txt "$test_dir/expected-output.txt"
echo "recorded $(wc -l < tape.txt) tape records into $test_dir"
