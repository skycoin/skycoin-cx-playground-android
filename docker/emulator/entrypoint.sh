#!/bin/sh
# Boots the rig AVD headless, reports the boot time, then forwards adb and
# the emulator console to ports that docker can publish.
set -eu

[ -c /dev/kvm ] || { echo "rig: /dev/kvm is missing, run with --device /dev/kvm" >&2; exit 1; }

start=$(date +%s%N)
emulator -avd rig -port 5554 -no-window -no-audio -no-boot-anim -no-snapshot \
	-gpu "${RIG_GPU:-swiftshader_indirect}" -accel on -camera-back none -camera-front none \
	${RIG_EMULATOR_FLAGS:-} &
emu=$!
trap 'kill $emu 2>/dev/null; wait $emu; exit 0' TERM INT

adb start-server >/dev/null 2>&1
timeout "${RIG_BOOT_TIMEOUT:-300}" sh -c '
	adb -s emulator-5554 wait-for-device
	until [ "$(adb -s emulator-5554 shell getprop sys.boot_completed 2>/dev/null | tr -d "\r")" = 1 ]; do
		sleep 0.5
	done' || { echo "rig: boot did not complete" >&2; kill $emu; exit 1; }
end=$(date +%s%N)
echo "rig: boot completed in $(( (end - start) / 1000000 )) ms"

socat tcp-listen:15555,fork,reuseaddr tcp:127.0.0.1:5555 &
socat tcp-listen:15554,fork,reuseaddr tcp:127.0.0.1:5554 &
touch /tmp/rig-ready
echo "rig: ready, adb on port 15555, console on port 15554"
wait $emu
