# CX Playground for Android

An Android app that lists CX programs and runs them. The UI is
[Gio](https://github.com/0magnet/gio), packaged with gogio from the
0magnet gio-cmd fork, the same stack as the skywire Android app.

```
make test     # unit tests, and every app in apps/ run headless twice in one process
make apk      # build/cx-playground.apk for arm64 and amd64
make install  # adb install and start
```

## Apps, manifests and hashes

An app is a directory of CX sources plus a `cxapp.json` with its name,
description, kind (`gfx` or `console`) and entry file. `cxmanifest` turns it
into a manifest:

```json
{
  "version": 1,
  "name": "Keyboard",
  "kind": "gfx",
  "entry": "main.cx",
  "chunk_size": 65536,
  "files": [
    {"path": "main.cx", "size": 2162, "sha256": "…", "chunks": ["…"]}
  ]
}
```

`cxmanifest build` and `hash` still print this JSON form of one app, and the
app shows its fields. The bundle the APK embeds uses the binary manifest of
docs/ProgramDistribution.md instead, in the layout of the program store:
`list.bin` is the signed manifest list, `m/<address>.bin` one signed binary
manifest per app, `p/<address>.pack` its linear packing and `index.txt` one
line per app. The app checks the list, every manifest and its signer, every
512-byte block and every file before it runs anything, and an app's hash is
its manifest address. The bundle key comes from a seed in `store`, so it only
marks the bundle as made by `cxmanifest`, and it is trusted for the bundle
alone. Signatures are not repeatable, so `cxmanifest bundle` keeps the
signature of each manifest whose address has not changed, and a rebuild of
the same apps gives the same bytes.

```
cxmanifest hash apps/keyboard
cxmanifest build apps/keyboard > manifest.json
cxmanifest bundle -o cmd/cxplayground/bundle apps/*
cxmanifest verify cmd/cxplayground/bundle
```

Two optional `cxapp.json` fields serve programs that read files. `args` is
the program's `os.Args`, as the `++` arguments of cx, and goes into the
manifest. `include` lists files and directories outside the app directory,
relative to it, whose files `cxmanifest` adds under their base names, so an
app can use sources kept elsewhere without a copy. The app writes the files
of an app that are not CX sources to `app-files/<hash>/` in its files dir
and makes that the program's sandbox root, which its `os` file operations
resolve against. `os.Exit` ends the program, not the app. skycoin-cx keeps
its system libraries (file, net and process) disabled unless a run enables
them, and the app enables all three for the bundled apps through
`cxrun.Options.Libraries`.

### Program forms

The `form` field of `cxapp.json` says how the app holds its program.

| `form` | The bundle holds | The app runs it |
| --- | --- | --- |
| none | the CX sources | through the parser, on the old executor |
| `ir` | `main.cxir`, the program serialized with skycoin-cx `ast.SerializeCXProgram`, the bytes `cx build` appends to its core | with `ast.Deserialize` and no parser, on the auto executor: the vm when every function lowers, the old one otherwise |
| `forth` | `main.fs`, Forth source | on the cx-forth VM, see below |

`make bundle` compiles the sources of an `ir` app with the pinned skycoin-cx
and keeps only the serialized program. The app registers the same opcodes in
the same order, the `app` package included, so the IR of one build of the
app runs in that build. A `forth` app with `"cxir_example": "fib"` gets the
cx-forth translation of that `cxir.Examples` program as its `main.fs`.

cx-forth is `github.com/skycoin/skycoin-cx-forth`, whose `forth` and `cxir`
packages build with the standard Go compiler. In a `gfx` app `forthrun` gives its
VM a terminal grid of the app font's cells as output, and these words. A
console app prints to the console and has no grid and no keys.

| Word | Does |
| --- | --- |
| `AT-XY ( col row -- )` | moves the cursor |
| `PAGE ( -- )` | clears the grid |
| `GRID-SIZE ( -- cols rows )` | the grid size |
| `KEY ( -- code )` | waits for a key, 10 for ENTER and 8 for backspace |
| `KEY? ( -- flag )` | whether a key is waiting |
| `SHOW ( -- )` | draws the grid now, as `KEY` and the end of the program do |

`store.Source` reads a bundle by path in that layout, and the program
loader below fetches programs over the network into the same layout.

### Programs over UDP

A program can also come from a server over UDP as a signed manifest and
512-byte blocks, as docs/ProgramDistribution.md describes. `programblocks`
holds the format, the server and the client, and `cmd/programblocks` serves a
directory of programs and fetches one. An app of kind `fetch` holds no
program: its `args` are the server address, the trusted signer key and the
program name, and the app fetches, verifies and runs that program.

```
programblocks signer -seed "a test seed"
programblocks serve -addr :7002 -seed "a test seed" DIR
programblocks list -addr HOST:7002 -signer PUBKEY
programblocks fetch -addr HOST:7002 -signer PUBKEY -out OUT -text PROGRAM
```

An app of kind `programs` opens the program loader instead: its `args` are
the server address and the signer keys to trust, which go to
`programs/trusted.txt` in the files dir. The screen lists the server's
programs (number, name, remote, partial or cached, `+ram` when loaded,
length) and offers download, validate, run, unload and clear for the
selected one. Downloads land in `programs/` block by block and resume after
a stop. Validate shows the `programcheck` report, and run starts only a
program the report allows, with the system libraries it allows. A `fetch`
app shows the same report and refuses the same programs.

### Programs by announce and request

`programnetwork` is the DHT prototype of #18. Nodes subscribe to each other
and announce each program they hold with a have packet. There is no flood:
a node puts every have it receives in its have queue, reads every one, and
requests only what its type keeps (#42). A super node requests everything
and relays it once, a medium node its pins and what fits its budget, and a
thin node is a one-shot get with no subscription (#44). Each node keeps a
peer list: it exchanges public peers as Bitcoin does (#41), and private
peers stay in its local file. `cmd/programnetwork` runs a node, gets a
program and edits the peer list file.

```
programnetwork node -addr :7010 -store DIR -publish PROGRAMS -seed "a test seed"
programnetwork node -addr :7011 -store DIR -signer PUBKEY -subscribe HOST:7010
programnetwork get -signer PUBKEY ADDRESS HOST:7010
programnetwork peers -file peers.txt add 192.168.1.5:7010 private
programnetwork peers -file peers.txt add 203.0.113.5:7010 public
programnetwork getpeers HOST:7010
```

An app of kind `subscribe` runs a node on the phone: its `args` are the
comma separated nodes to subscribe to, the trusted signer key and the name
in the wanted program's `cxapp.json`. It reads every have, requests the
program, verified block by block, then checks and runs it like a `fetch`
app. An app of kind `thin` gets one program by address from one node.

### Checking a program before it runs

`programcheck` reads a CX program without running it, compiled from source
or loaded from its serialized form. It lists the system library calls
(file, net, process), the `app.NetDial` networks (udp, tcp, or unknown when
the name is not a constant), the imports, the opcode sets (core, app from
opcode 4096, library) and IR problems such as a call of a missing function.
A policy says what is allowed. `PolicyFetched`, for programs from the
network, blocks the `os` and `tcp` imports and every system library, and
`PolicyBundled` allows everything. The report is one monospaced line per
check:

```
file system no
networking yes, udp yes, tcp no
process no
imports app string time
blocked imports none
sandbox violations none
verdict run
```

## The `app` package for CX

Programs draw a display list that the host renders with Gio's GPU renderer
(OpenGL ES on Android). CX does not issue GL calls itself. Programs import the
package with `import "sys/app"`.

| Function | Purpose |
| --- | --- |
| `app.ScreenSize() (w, h i32)` | canvas size in pixels |
| `app.Clear(r, g, b)` | start a frame |
| `app.Rect`, `app.Circle`, `app.Line`, `app.Tri` | shapes, colours as r, g, b, a in 0..1 |
| `app.Text(s, x, y, size, r, g, b, a)` | draws in the app font at its one size, `size` is ignored |
| `app.Frame()` | show the frame, paced by the display |
| `app.PollEvent() (kind i32, x, y f32, code i32)` | `app.TOUCH_DOWN`, `TOUCH_MOVE`, `TOUCH_UP`, `KEY`, `RESIZE`, or `app.NONE` |
| `app.ShowKeyboard()`, `app.HideKeyboard()` | the soft keyboard |
| `app.ShouldClose() bool` | the user pressed back |
| `app.Rune(code i32) str` | a key code as text, as `sprintf` has no `%c` |
| `app.CellSize() (w, h f32)` | the advance and line height of one character of the app font, the cell of a terminal grid |
| `app.NetDial(network, address str) (conn i32, err str)` | a `tcp` or `udp` connection, `err` is "" when it worked |
| `app.NetSend(conn i32, data str) (err str)` | writes data, one datagram for UDP |
| `app.NetRecv(conn i32, timeoutMs i32) (data str, err str)` | what arrived, one datagram for UDP, `err` is `timeout` when nothing did |
| `app.NetClose(conn i32)` | closes it, as the end of the program does |
| `app.FileSync(file i32) (ok bool)` | flushes a file from `os.Open` or `os.Create` to storage, as the `os` package of skycoin-cx has no sync |

Console output from `printf` and `string.PrintLine` is shown on screen, and so
are compile and runtime errors.

## Emulator test rig

`docker/emulator` is a headless Android emulator image (x86_64, no Google
Play, KVM). `cmd/emulator-driver` drives the app through adb, so it also
works against a phone with `-serial`.

```
make emulator-image   # build the image
make emulator-smoke   # boot it, build the APK, run tests/, print the timing table
```

Each folder under `tests/` is one test. A folder with an `app/` or an
`emulator.txt` is an emulator test, and the driver skips the others, which
hold go tests. `app/` is the CX app, which `make bundle` adds to the APK.
`input.txt` lists the steps and `expect.txt` the checks. Results go to
`results/<time>/<test>/`: the PNGs, pulled files, the logcat and the timing
table, and `results.txt` with the result lines of a test program, which
the suite report repeats.

| `input.txt` step | Does |
| --- | --- |
| `text <string>`, `key <NAME or code>` | types, `key ENTER` or `key KEYCODE_7`, a bare number is a key code |
| `tap <x> <y>`, `swipe <x1> <y1> <x2> <y2> [ms]` | touch, in screen pixels of the 720x1280 rig display |
| `rotate <0..3>` | turns the display with `settings put system user_rotation`, which works on a phone too |
| `screenshot <name>` | saves the screen |
| `wait <ms>`, `wait-for <text>` | pauses, or waits for a test mode line containing text |
| `keyboard shown\|hidden` | waits for that soft keyboard state in `dumpsys input_method` |
| `pull <path> <name>` | saves a file the app published in test mode, path relative to its files dir, globs allowed |

Each injected step is timed until the frame that reflects it, `rotate` until
the frame after its resize event.

Before the first step the driver waits until input reaches the program: the
app's window has focus in its own process (`mHasWindowFocus` in `dumpsys
activity top`) and, for an app that draws frames, the canvas holds key focus
(the app's `input focused=true` line). Then, if the display is in touch mode,
it sends PAGE_UP to the app, see "Known limits". After a boot it also waits
until the home screen has focus and SystemUI has turned on its theme
overlays, which relaunches every app.

| `expect.txt` line | Passes when |
| --- | --- |
| any other text | a test mode line contains it, after the line the previous one matched |
| `screen-changed <a> <b>` | two screenshots differ |
| `screen-matches <name> [x y w h]` | the screenshot, or the region, matches `expected/<name>.png` within `-tolerance` per channel and `-max-diff` of the pixels |
| `file-equals <pulled> <path>` | a pulled file equals a file of the repository, path relative to the test folder |
| `file-contains <pulled> <text>` | a pulled file contains the text |
| `result <name> <verdict>` | the test program printed the result line `name` with that verdict, see "Test programs" and package `testreport` |
| `results-pass` | every result line the program printed is `true` or `skipped`, and there is one |

Regions keep the status bar clock and the top bar with the app hash out of
the comparison. `suite -update-references` writes the captured regions to
`expected/` instead of comparing them, for a new test or a deliberate change.

`emulator.txt` holds optional settings:

| Line | Does |
| --- | --- |
| `app <name>` | runs a bundled app from `apps/` instead of `app/` |
| `ready <text>` | waits for this test mode line instead of the first frame, for apps that draw no CX frames |
| `setprop <name> <value>` | sets a property before the start and clears it after |
| `before <command>`, `after <command>` | device shell commands around the test |
| `timeout <seconds>` | for `wait-for` and the checks, 10 by default |
| `input untimed` | sends the `input.txt` events without timing them, for a screen of the app that is not a program canvas |
| `echo-server <port>` | runs `net-echo-server` on that TCP and UDP port in the rig container, which the emulator reaches as 10.0.2.2 |
| `program-server <port> <dir> <seed>` | runs `programblocks serve` on that UDP port in the rig container for the program directories under `<dir>` of the test folder, signed with the key derived from `<seed>` |
| `program-nodes <port> <count> <dir> <seed>` | runs `count` `programnetwork` nodes on UDP ports `port` and up in the rig container: the first publishes the program directories under `<dir>` of the test folder, signed with the key derived from `<seed>` and dated 2026-11-24, each other subscribes to the one before and requests what it reads in its have queue, and the app starts once the last node holds every program |
| `program-node-type <type>` | the nodes' `-type`, `medium` or `super`, super by default |
| `netem <args>` | `tc qdisc ... netem <args>` on the packets of that port in both directions, for example `delay 200ms loss 30%` |
| `capture <file>` | `tcpdump` of the packets of that port, pulled as `<file>` for the `file-` checks |

The network settings act on the container's loopback, where the emulator's
user mode network meets the echo server, so they need the rig container
(`--cap-add NET_ADMIN`, and `iproute2` and `tcpdump` in the image) and do not
work against a phone. A TAP device for the emulator (`-net-tap`) would replace
that user mode network, and the Android image would then need DHCP, DNS and a
router on the TAP side as well. The emulator console's `network delay` and
`network speed` have no loss. tc netem on the container side gives delay, loss
and capture of the same packets with none of that, so the rig uses it.

The app enters test mode when the system property `debug.cxplayground.test`
is 1, and `debug.cxplayground.autostart` names an app to run at launch. Test
mode logs each frame the program draws, the last event the program polled
before drawing it, the texts it shows, each queued event with a sequence
number and each line of console output, all starting with `cxtest`. It also
saves the standard output of each program to `cxtest/stdout.txt` in the
app's files dir. It publishes that file and the benchmark results it saves
through logcat as base64 lines with a SHA-256, since `run-as` cannot read
the files of the release APK that gogio builds. `debug.cxplayground.benchmark.runs` starts the benchmark
suite at once with that many runs.

| Test | Checks |
| --- | --- |
| `type-echo` | text, a key and a tap reach the program and its frames |
| `terminal-grid-text` | text at rows and columns of the grid from `app.CellSize`, a block cursor that typing moves |
| `terminal-grid-scroll` | a full grid scrolls up one row per line |
| `terminal-grid-resize-on-rotation` | the grid takes 70 by 10 cells in landscape and 42 by 21 again in portrait |
| `opengl-scene-reference` | a fixed scene of rectangles, circles, a triangle and lines against a reference image |
| `ui-key-input` | single key events, letters, digits, ENTER and DEL |
| `ui-text-input` | text with spaces and punctuation, and backspace |
| `ui-tap` | taps arrive at the right canvas pixels |
| `ui-swipe` | swipes in four directions |
| `ui-soft-keyboard` | `app.ShowKeyboard` and `app.HideKeyboard` open and close the soft keyboard |
| `ui-rotation` | resize events on a turn and a tap in landscape |
| `benchmark-suite-one-pass` | one pass of the benchmark suite, every benchmark passes and the JSON document is saved |
| `femtoclaw-replay-in-emulator` | the femtoclaw CX port replays its tape in the app with output byte identical to the host replay |
| `benchmark-hashes` (go) | expected hashes against the math and skycoin-cx, and every program run from the bundle on both executors prints them |
| `benchmark-opcounts` (go) | the operation counts in `suite.json` (needs `-tags cxopcount`) |
| `results-schema-roundtrip` (go) | a document encodes and decodes unchanged, malformed ones are refused |
| `collector-accept-reject` (go) | the collector stores a valid upload and refuses duplicates, bad and oversized ones |
| `femtoclaw-replay-in-app-runtime` (go) | the bundled femtoclaw app run through `store` and `cxrun` on the host prints the host replay's bytes, twice |
| `ui-enter-first-key` | ENTER as the first key after the start reaches the program |
| `cx-source-fib15`, `cx-ir-fib15`, `forth-cxir-fib15` | one program, fib 0 to 14, as CX source, as serialized CX and as the cx-forth translation of its CX IR, each printing the same file |
| `cx-source-primes`, `cx-ir-primes` | the primes benchmark as source on the old executor and serialized on the vm, printing the same file |
| `cx-ir-key-input` | the `ui-key-input` program serialized, with the checks of `ui-key-input` |
| `forth-grid-text` | Forth text at grid rows and columns, `KEY` waits for ENTER |
| `forth-key-echo` | typed keys reach Forth `KEY` and are echoed line by line |
| `net-tcp-echo` | five lines through the TCP echo server on the host |
| `net-udp-echo` | 20 datagrams through the UDP echo server, no loss and no delay |
| `programs-loader-screen-in-emulator` | the loader lists two programs from the rig, downloads, validates and runs one, and refuses the other, which reads a file |
| `net-udp-delay-loss` | the same program with 200 ms delay and 30% loss each way, reports both, and the capture holds its datagrams |
| `cx-forms-same-output` (go) | the bundled apps of each form print the expected files on the host |
| `net-echo-on-host` (go) | the network programs against `netecho` on the host |
| `program-validation-library-calls` (go) | one program per system library is reported with its call, allowed when bundled and refused when fetched |
| `program-validation-network-udp` (go) | the UDP echo app reports udp, the TCP echo app tcp, a network from the arguments unknown |
| `program-validation-blocked-import` (go) | a fetched program importing `os` is refused |
| `program-validation-opcode-sets` (go) | an app opcode in a core-only program fails, and the serialized form checks the same as the source |
| `program-validation-report-text` (go) | the report text of two programs matches the expected lines |
| `program-loader-store-resume` (go) | a download stopped part way resumes from the kept blocks, asking only for the missing ones; an untrusted list is refused |
| `program-loader-lister` (go) | `index.txt` of 10 and 1000 programs reads back row for row with the same number of allocations |
| `program-loader-memory` (go) | load, unload and reload from RAM give the same files, a limit of one keeps the last program |
| `program-test-suite-report-format` (go) | result lines read back unchanged, malformed ones are refused, and both test programs run from the bundle print only passing lines |
| `program-test-suite-hash-known` | the SHA-256 and Merkle known answer tests of the hash and memory program |
| `program-test-suite-memory-patterns` | the memory patterns and the flipped bit of the hash and memory program |
| `program-test-suite-storage-file-system` | method A of the storage program at 512 to 4096 bytes |
| `program-test-suite-storage-direct-block` | method B (D29) of the storage program at 512 to 4096 bytes, and the flipped byte |
| `program-test-suite-serial-skip-on-phone` | the four serial tests report `skipped` |

## Test programs

Two bundled CX apps test the device and report one result line per test
(#19). "Test program: hash and memory" (`apps/test-hash-memory`) checks known
hashes and memory patterns. "Test program: storage and serial"
(`apps/test-storage-serial`) writes, reads and verifies storage blocks and
reports the serial tests. The line, read and written by package
`testreport`, is

```
name verdict blocks_per_s kb_per_s latency_us [serial_read_kb_s serial_write_kb_s] [note]
```

The verdict is `true`, `false` or `skipped`. A measurement is a decimal
number, or `-` when the test did not measure it. The two serial speeds are
present exactly when the name starts with `Serial`, and the note runs to the
end of the line. Only the measured work counts in the time: the pattern
generation and the comparison of a storage test are left out.

| Test | Does |
| --- | --- |
| `HashSha256Known` | SHA-256 written in CX, as skycoin-cx has none, over the FIPS 180-4 vectors "", "abc" and the 448-bit message and 16 KiB of a pattern, a block is 64 bytes |
| `HashMerkleKnown` | the Merkle hash of `programblocks.MerkleHash` over five 512-byte blocks |
| `MemoryPatternAddress`, `MemoryPatternWalkingOnes`, `MemoryPatternWalkingZeros`, `MemoryPatternMovingInversions` | the patterns of skycoin-cx `t0-benchmarks/compare/cx/memtest.cx` over 128 KiB, a block is a 4-byte word |
| `MemoryPatternRandomSeeded` | a seeded xorshift32 sequence written and generated again to compare |
| `MemoryCorruptionDetectedBitFlip` | flips one bit after the write and passes when the read back finds exactly one word |
| `StorageFileSystemWrite<size>`, `StorageFileSystemRead<size>` | method A: 64 blocks written one after another to one file per block size, then read back and verified |
| `StorageDirectBlockWrite<size>`, `StorageDirectBlockRead<size>` | method B, decision D29: 64 blocks at fixed offsets of one preallocated file, with `app.FileSync` after every write, then read back and verified |
| `StorageCorruptionDetectedByteFlip` | flips one byte of a written block on storage and passes when the read back finds exactly one |
| `SerialUartRead`, `SerialUartWrite`, `SerialSpiRead`, `SerialSpiWrite` | `skipped`, the phone has no UART or SPI |

The sizes are 512, 1024, 2048 and 4096 bytes. The files live in the app's
sandbox directory, which the app clears before every run.

## Benchmark suite

The "Benchmark suite" app (`apps/benchmarks`, kind `benchmark`) runs the six
programs of skycoin-cx `t0-benchmarks/compare` (count, fib, array, struct,
memtest, primes) on the old executor, which the other apps use, and on the vm.
Each program runs N times per executor (5 by default, `fewer` and `more`
change it before `start`). The app then shows the results document, saves it
to `benchmark-results/<time>-<run_id>.json` in the app's files dir and offers
to upload it.

The choices below were made without a spec and are provisional.

- **Scale.** The phone scale is about 1/30 of the full skycoin-cx scale, so one
  run of the suite on the old executor takes a few seconds on a desktop. The
  sizes are in `apps/benchmarks/suite.json`. The expected hashes there are
  derived from the math by `benchsuite.ExpectedHash`, a copy of
  `compareExpected` in skycoin-cx `t0-benchmarks/compare_test.go`. Tests check
  them against the math and the copy against skycoin-cx `expected.txt`.
- **Time.** Wall time of `main` only. Parsing and compiling are left out. The
  median of the N runs is reported, and every run is kept in `runs_ms`.
- **ops/ms.** The app is not built with the `cxopcount` tag, because counting
  costs time in the loop. The operation counts of each program are measured
  once on a host and stored in `suite.json`, and
  `go test -tags cxopcount ./tests/benchmark-opcounts` checks them (`make test`
  runs it). The old executor counts expressions and the vm counts
  instructions, so ops/ms is only comparable within one executor.
- **Upload.** POST of the document to `BENCHMARK_UPLOAD_URL`, set at build time
  with `make apk BENCHMARK_UPLOAD_URL=https://host/runs`. It is empty by
  default, and then the app says that upload is disabled. Release APKs
  upload to the collector below.
- **Device profile.** Android release and API level, manufacturer, model and
  ABI from `getprop`, core count, highest `cpuinfo_max_freq`, `MemTotal`, and
  the app, skycoin-cx and Go versions. The only identifier is a random run ID.
  No serial, IMEI, account or location is read.

### Results document

```
{
  "schema": "cx-playground-benchmark/1",
  "run_id": "32 hex digits, random per run",
  "started_at": "RFC 3339 UTC", "finished_at": "RFC 3339 UTC",
  "build": {"app_version", "app_revision"?, "skycoin_cx", "go_version"},
  "device": {"os", "arch", "android_release"?, "android_sdk"?, "manufacturer"?,
             "model"?, "abi"?, "cpu_cores", "cpu_max_mhz", "ram_total_mb"},
  "suite": {"scale", "sizes": {"count_n", "fib_n", "array_rounds",
            "struct_rounds", "mem_n", "mem_walk", "sieve_n", "miller_n",
            "lucas_rounds"}, "runs"},
  "results": [{
    "benchmark": "count|fib|array|struct|memtest|primes",
    "executor": "old|vm",
    "status": "pass|fail|rejected|error",
    "runs_ms": [n, ...], "median_ms": n,
    "hash": "decimal FNV-1a the program printed",
    "expected_hash": "decimal",
    "operations"?: n, "ops_per_ms"?: n,
    "message"?: "why it failed, was rejected or errored"
  }]
}
```

Fields marked `?` may be left out. Hashes are strings because they do not fit
a JSON number. `pass` needs the expected hash and the `<name> pass` line,
`rejected` means the vm does not lower the program. `benchsuite.Validate` is
the full rule set and the collector applies it.

### Collector

`cmd/benchmark-collector` takes `POST /runs`, validates the document, stores it
in a bbolt file keyed by run ID and lists all runs at `GET /runs.txt` (plain
text) and `GET /` (HTML).

```
make collector
build/benchmark-collector -addr 127.0.0.1:8090 -db benchmark-runs.db
```

It runs at https://bench.skywire.dev on prod02, and release APKs are built
with `make apk BENCHMARK_UPLOAD_URL=https://bench.skywire.dev/runs`. The
service is `cx-benchmark-collector.service`, which runs
`/usr/local/bin/cx-benchmark-collector` on `127.0.0.1:8090` with a dynamic
user and stores the runs in `/var/lib/cx-benchmark-collector/runs.db`.
Caddy proxies the host to it with a 1 MB request body limit.

To redeploy, build with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o
cx-benchmark-collector ./cmd/benchmark-collector`, copy the binary to the
host, install it over `/usr/local/bin/cx-benchmark-collector` and run
`systemctl restart cx-benchmark-collector`. The runs file stays.

## Known limits

A program cannot be interrupted. Back stops a `gfx` program at its next
`app.ShouldClose()`; a console program runs to the end first.

A boot or a tap puts the display in touch mode, and Android consumes the
first navigation key that takes a window out of it (ENTER, a DPAD key, TAB,
PAGE_UP, in `ViewRootImpl.checkForLeavingTouchModeAndConsume`). So an ENTER
sent first to an app started in touch mode never reached the program. Before
the first step the driver sends PAGE_UP, which the app ignores, to spend that.
A key sent to the home screen does not work for this, as its window may not
know that the display is in touch mode. Keys sent before the app's window has
focus in its own process, or while SystemUI relaunches every app after a
boot, are dropped too, which the driver's waits cover.

## Look

The whole UI, CX `app.Text` included, is JetBrains Mono at one size on a
dark background, plain text lines with no boxes. The font is under the SIL
Open Font License 1.1, see `cmd/cxplayground/font/OFL.txt`.
