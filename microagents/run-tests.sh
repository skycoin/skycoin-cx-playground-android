#!/bin/sh
# run-tests.sh builds the rig tools into build/ and runs every test folder.
#
# cx comes from $CX when set, else it is built from the skycoin-cx checkout
# in $SKYCOIN_CX_DIR, else installed with go install at $SKYCOIN_CX_VERSION.
# The harnesses of the originals need a C compiler and libcjson.
# TESTS is a glob of test folders below this directory, default */tests/*.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
build="$here/build"
mkdir -p "$build"
export GOWORK=off

(cd "$here" && go build -o "$build/backendstore" ./backendstore && go build -o "$build/portlayout" ./portlayout)

cjson=$(pkg-config --cflags --libs libcjson 2>/dev/null || echo -lcjson)

# build_spiffs_agent AGENT [SOURCE...] builds the host harness of femtoclaw or
# mimiclaw, which share their file layout, with extra upstream sources.
build_spiffs_agent() {
	agent=$1
	shift
	original="$here/$agent/original/main"
	extra=""
	for source in "$@"; do extra="$extra $original/$source"; done
	${CC:-cc} -std=gnu11 -O1 -Wall -Wno-unused-function -Wno-format-truncation -Wno-unused-but-set-variable \
		-include "$here/$agent/before/shim/host_shim.h" -I"$here/$agent/before/shim" -I"$original" \
		-o "$build/$agent-before" "$here/$agent/before/harness.c" \
		"$original/agent/context_builder.c" "$original/llm/llm_proxy.c" \
		"$original/memory/session_mgr.c" "$original/memory/memory_store.c" \
		"$original/tools/tool_registry.c" "$original/tools/tool_get_time.c" \
		"$original/tools/tool_files.c" "$original/skills/skill_loader.c" $extra \
		$cjson
}
build_spiffs_agent femtoclaw tools/tool_send_message.c
build_spiffs_agent mimiclaw

# zclaw builds on its upstream host test mocks. memory_list (it needs the NVS
# iterator), get_diagnostics and the GPIO handlers are renamed away, and the
# harness answers them as not available.
zclaw="$here/zclaw/original"
zclaw_flags="-std=gnu11 -O1 -Wall -DTEST_BUILD -include $here/zclaw/before/shim/host_shim.h -I$here/zclaw/before/shim -I$zclaw/main -I$zclaw/test/host"
${CC:-cc} $zclaw_flags -Dtools_memory_list_handler=tools_memory_list_handler_not_built \
	$(pkg-config --cflags libcjson 2>/dev/null) -c "$zclaw/main/tools_memory.c" -o "$build/zclaw-tools-memory.o"
${CC:-cc} $zclaw_flags -Dtools_get_diagnostics_handler=tools_get_diagnostics_handler_not_built \
	$(pkg-config --cflags libcjson 2>/dev/null) -c "$zclaw/main/tools_system.c" -o "$build/zclaw-tools-system.o"
${CC:-cc} $zclaw_flags -Dtools_gpio_write_handler=tools_gpio_write_handler_not_built \
	-Dtools_gpio_read_handler=tools_gpio_read_handler_not_built -Dtools_gpio_read_all_handler=tools_gpio_read_all_handler_not_built \
	$(pkg-config --cflags libcjson 2>/dev/null) -c "$zclaw/main/tools_gpio.c" -o "$build/zclaw-tools-gpio.o"
${CC:-cc} $zclaw_flags -o "$build/zclaw-before" "$here/zclaw/before/harness.c" "$build/zclaw-tools-memory.o" "$build/zclaw-tools-system.o" "$build/zclaw-tools-gpio.o" \
	"$zclaw/main/agent.c" "$zclaw/main/agent_commands.c" "$zclaw/main/agent_prompt.c" \
	"$zclaw/main/json_util.c" "$zclaw/main/local_admin.c" "$zclaw/main/boot_guard.c" \
	"$zclaw/main/tools.c" "$zclaw/main/tools_common.c" "$zclaw/main/gpio_policy.c" \
	"$zclaw/main/tools_persona.c" "$zclaw/main/memory_keys.c" "$zclaw/main/ratelimit.c" \
	"$zclaw/test/host/mock_memory.c" "$zclaw/test/host/mock_freertos.c" \
	"$zclaw/test/host/mock_esp.c" "$zclaw/test/host/mock_user_tools.c" "$zclaw/test/host/mock_system_diag_deps.c" \
	$cjson

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
export MIMICLAW_BEFORE="$build/mimiclaw-before"
export ZCLAW_BEFORE="$build/zclaw-before"

failed=0
if output=$(cd "$here" && go test ./... 2>&1); then
	echo "PASS go-test: unit tests of backendstore, and testgen and codingbench build"
else
	echo "FAIL go-test"
	echo "$output" | sed 's/^/    /'
	failed=1
fi
for agent in femtoclaw mimiclaw zclaw; do
	if output=$(cd "$here" && go run ./testgen -agent $agent -check 2>&1); then
		echo "PASS $agent/generated-folders: $output"
	else
		echo "FAIL $agent/generated-folders: $output"
		failed=$((failed + 1))
	fi
done
for test in "$here"/${TESTS:-*/tests/*}/; do
	name=$(basename "$(dirname "$(dirname "$test")")")/$(basename "$test")
	if output=$("$test/run.sh" 2>&1); then
		echo "PASS $name: $(echo "$output" | tail -1)"
	else
		echo "FAIL $name"
		echo "$output" | sed 's/^/    /'
		failed=$((failed + 1))
	fi
done
(cd "$here" && go build -o "$build/codingbench" ./codingbench)
for run in "$here"/codingbench/runs/*/; do
	name=codingbench/$(basename "$run")
	if output=$(cd "$here" && "$build/codingbench" -mode replay -run "$run" 2>&1); then
		echo "PASS $name: replay of every tape, $(echo "$output" | tail -1)"
	else
		echo "FAIL $name"
		echo "$output" | sed 's/^/    /'
		failed=$((failed + 1))
	fi
done
[ "$failed" -eq 0 ] || { echo "$failed test(s) failed"; exit 1; }
echo "all tests passed"
