#!/bin/sh
# run-tests.sh builds the rig tools into build/ and runs every test folder.
#
# cx comes from $CX when set, else it is built from the skycoin-cx checkout
# in $SKYCOIN_CX_DIR, else installed with go install at $SKYCOIN_CX_VERSION.
# The original femtoclaw harness needs a C compiler and libcjson.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
build="$here/build"
mkdir -p "$build"
export GOWORK=off

(cd "$here" && go build -o "$build/backendstore" ./backendstore && go build -o "$build/portlayout" ./portlayout)

original="$here/femtoclaw/original/main"
${CC:-cc} -std=gnu11 -O1 -Wall -Wno-unused-function -Wno-format-truncation \
	-include "$here/femtoclaw/before/shim/host_shim.h" -I"$here/femtoclaw/before/shim" -I"$original" \
	-o "$build/femtoclaw-before" "$here/femtoclaw/before/harness.c" \
	"$original/agent/context_builder.c" "$original/llm/llm_proxy.c" \
	"$original/memory/session_mgr.c" "$original/memory/memory_store.c" \
	"$original/tools/tool_registry.c" "$original/tools/tool_get_time.c" \
	"$original/tools/tool_files.c" "$original/skills/skill_loader.c" \
	$(pkg-config --cflags --libs libcjson 2>/dev/null || echo -lcjson)

if [ -z "${CX:-}" ]; then
	if [ -n "${SKYCOIN_CX_DIR:-}" ]; then
		(cd "$SKYCOIN_CX_DIR" && go build -o "$build/cx" ./t11-cx)
	else
		GOBIN="$build" go install "github.com/skycoin/skycoin-cx/t11-cx@${SKYCOIN_CX_VERSION:-main}"
		mv "$build/t11-cx" "$build/cx"
	fi
	CX="$build/cx"
fi
export CX
export BACKENDSTORE="$build/backendstore"
export PORTLAYOUT="$build/portlayout"
export FEMTOCLAW_BEFORE="$build/femtoclaw-before"

failed=0
for test in "$here"/*/tests/*/; do
	name=$(basename "$(dirname "$(dirname "$test")")")/$(basename "$test")
	if output=$("$test/run.sh" 2>&1); then
		echo "PASS $name: $(echo "$output" | tail -1)"
	else
		echo "FAIL $name"
		echo "$output" | sed 's/^/    /'
		failed=$((failed + 1))
	fi
done
[ "$failed" -eq 0 ] || { echo "$failed test(s) failed"; exit 1; }
echo "all tests passed"
