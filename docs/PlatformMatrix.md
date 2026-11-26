# CX platform matrix

This page says which CX capabilities are tested on which platform, by which
test folder, and why a cell is not possible. Every test is one folder. The
skycoin-cx version is the release v0.9.0-alpha1 (commit ea4f4aa2) on every
platform. The results are those of 2026-11-25.

Platforms and where their tests live:

- Desktop Linux: `desktoptests/` (the release archive) and `microagents/`
  (`run-tests.sh` with the release cx), in this repository.
- Desktop macOS and Windows: build only. The release archives exist, cx
  cross-compiles for darwin and windows on amd64 and arm64, and skycoin-cx
  CI runs its own tests on macos-latest and windows-latest. The tests of
  this page did not run there.
- wasm on Node: `microagents/` with `CX=testlib/cx-wasm.sh`, cx built for
  js/wasm.
- Browser: `browsertest/tests/` of skycoin-cx-playground, headless Brave
  driven over the DevTools protocol, the page served with cross-origin
  isolation.
- Android: `tests/` of this repository on the emulator rig
  (`make emulator-smoke`), the x86_64 APK on Android 15 (SDK 35) in docker. All 38 emulator tests passed.

PASS means the test ran and passed. NP means not possible on that
platform, with the reason. Open means it needs a decision first.

| Capability | Desktop Linux | wasm on Node | Browser | Android |
| --- | --- | --- | --- | --- |
| CX source runs | PASS `cx-source-fib15` | PASS (micro agent ports) | PASS `cx-source-typed-run` | PASS `cx-source-fib15`, `cx-source-primes` |
| CX IR form runs with no parser | PASS `cx-ir-fib15` | NP: cx build needs a core binary, see the browser | PASS `cx-ir-build-and-run`, IR built in the page and on the host | PASS `cx-ir-fib15`, `cx-ir-primes`, `cx-ir-key-input` |
| Forth form of the CX IR | PASS `forth-cxir-fib15` | not run | PASS `forth-repl-typed-lines` | PASS `forth-cxir-fib15`, `forth-grid-text`, `forth-key-echo` |
| Key and UI events in, text grid and image out | NP: the release has no GUI (cxfx is out); stdin lines instead | NP: no screen | PASS `cx-source-typed-run`, `ui-example-select-keyboard`, `forth-repl-typed-lines` | PASS `type-echo`, `terminal-grid-text`, `terminal-grid-scroll`, `terminal-grid-resize-on-rotation`, `ui-key-input`, `ui-text-input`, `ui-tap`, `ui-swipe`, `ui-rotation`, `ui-soft-keyboard`, `ui-enter-first-key`, `opengl-scene-reference`, through adb input, logcat lines and screencap |
| Typed lines reach a running CX program | PASS `cx-stdin-lines` | not run | PASS `cx-stdin-typed-lines` | PASS `forth-key-echo`, `ui-text-input` (app.PollEvent, no stdin) |
| Micro agents replay (femtoclaw, mimiclaw, zclaw) | PASS `*/tests/replay-determinism`, `before-after` | PASS, same folders | PASS `microagent-*-replay` | PASS `femtoclaw-replay-in-emulator`, output equal to the host replay |
| 244 generated tool, pair and chain tests | PASS | PASS | not run, the desktop and Node runs cover them | NP: needs the C originals on the host |
| Coding benchmark, scripted stub tapes | PASS `codingbench/scripted-stub` | PASS | PASS `codingbench-<agent>-<task>`, 24 tests | not run |
| Model provider switch and memoizing on or off | PASS `providers/tests/*`, backendstore tests | NP: backendstore is a host service | NP: replay only, decision D137 | NP: replay only, decision D137 |
| Live model | pending: no API key on the machine | NP | NP, D137 | NP, D137 |
| TCP on loopback in one program | PASS `cx-net-tcp-loopback`, live and replayed | PASS by hand, skycoin-cx tests/test-net-tcp.cx, no committed folder | PASS `cx-net-tcp-loopback`, Go's in-memory network | not run, the echo tests cover TCP |
| UDP on loopback in one program | PASS `cx-net-udp-loopback` | PASS by hand, test-net-udp.cx, no committed folder | PASS `cx-net-udp-loopback` | not run, the echo tests cover UDP |
| TCP and UDP to another process | PASS `cx-net-tcp-echo-process`, `cx-net-udp-echo-process` | NP: in-memory network | NP: a page has no sockets, decision D134 | PASS `net-tcp-echo`, `net-udp-echo`, through the echo server in the rig container |
| UDP with delay and loss | not run | NP | NP | PASS `net-udp-delay-loss`, 200 ms and 30% loss each way with tc netem |
| programnetwork announce and request (the "DHT prototype of #18") | PASS `programnetwork-publish-get-subscribe`, between processes | NP | NP: Go, not CX, and no sockets | PASS `program-network-announce-request-in-emulator`, `program-network-thin-fetch-in-emulator` |
| Shared data sync, CXO over dmsg or skynet | Open, decision D133 | Open | Open | Open |
| CX net functions over dmsg | NP: not wired | NP | Open, decision D134 | NP |

## Notes

- In the browser, cx.wasm runs in a bottle process in a Worker. "run with
  input" runs it on the page thread instead, because a Worker process reads
  EOF on stdin. `cxcorefile` is the IR runner for js/wasm, since a core
  there cannot read its own file.
- Go's js/wasm network is an in-memory network inside one process. That is
  why loopback passes in wasm and nothing reaches another machine. Skywire's
  dmsg client runs in js/wasm over WebSocket. Carrying the CX net functions
  over it needs a hook in skycoin-cx t1-netbatch (D134).
- No DHT exists in these repositories. programnetwork is an announce and
  request network over UDP. Skywire uses CXO over dmsg and skynet for shared
  data (D133).
- The model provider and memo switches are flags of `backendstore serve`,
  `codingbench` and `record.sh`, see microagents/README.md.
