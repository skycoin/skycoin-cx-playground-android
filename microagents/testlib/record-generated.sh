#!/bin/sh
# record-generated.sh AGENT TEST_DIR... records generated tests with the
# agent's record.sh. Their store is filled from the tape when they run, so
# the recorded store.bolt is not kept.
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
agent=$1
shift
for test_dir in "$@"; do
	"$here/$agent/record.sh" "$test_dir"
	rm -f "$test_dir/store.bolt"
done
