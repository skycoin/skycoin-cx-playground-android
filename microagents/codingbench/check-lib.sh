# check-lib.sh is sourced by the check.sh of each task. check.sh runs in the
# task's result tree, the files the agent left below /spiffs, with $CX set.

fail() { echo "not solved: $*"; exit 1; }

# expect_output EXPECTED FILE... runs the CX files and compares stdout.
expect_output() {
	expected=$1
	shift
	for f in "$@"; do [ -f "$f" ] || fail "$f is missing"; done
	got=$("$CX" --enable=file,net,process "$@" 2> cx-stderr.txt) || fail "cx $* failed: $(grep -v '^warning' cx-stderr.txt | head -3)"
	rm -f cx-stderr.txt
	[ "$got" = "$expected" ] || fail "cx $* printed '$got', want '$expected'"
}
