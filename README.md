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

Each file is split into `chunk_size` pieces, the last one shorter, and
`chunks` lists the SHA-256 of each piece in order. The app hash is the
SHA-256 of the manifest bytes exactly as encoded. The home screen lists one
line per app, and an opened app shows its hash and manifest details.

A bundle is content addressed: `index.txt` lists app hashes, `m/<hash>.json`
holds manifests and `c/<hash>` holds chunks. The app embeds the bundle built
from `apps/` and checks every manifest against its hash and every chunk,
file size and file hash before it runs anything.

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
resolve against. `os.Exit` ends the program, not the app.

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

Fetching apps over the network is deferred. `store.Source` is the seam: a
loader that can return an index, a manifest and a chunk by hash plugs in
without changes to verification or the UI.

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
table.

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
| `cx-forms-same-output` (go) | the bundled apps of each form print the expected files on the host |

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
  default, and then the app says that upload is disabled. No server is
  chosen yet.
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

It is not deployed anywhere. Where it runs and which URL the app uploads to
are open.

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
