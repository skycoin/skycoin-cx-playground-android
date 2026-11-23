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

Each folder under `tests/` is one test. `app/` is the CX app, which `make
bundle` adds to the APK. `input.txt` lists the events to inject and the
screenshots to take, and `expect.txt` the test mode lines that must appear in
logcat, in order. Results go to `results/<time>/<test>/`: the PNGs, the
logcat and the timing table.

The app enters test mode when the system property `debug.cxplayground.test`
is 1, and `debug.cxplayground.autostart` names an app to run at launch. Test
mode logs each frame the program draws, the last event the program polled
before drawing it, the texts it shows, each queued event with a sequence
number and each line of console output, all starting with `cxtest`.

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
  `go test -tags cxopcount ./gotests/benchmark-opcounts` checks them (`make test`
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

### Tests

One folder per test under `gotests/`. They are go tests, kept apart from
`tests/`, where the emulator driver expects every folder to hold an `app/`:

| Folder | Checks |
| --- | --- |
| `benchmark-hashes` | expected hashes against the math and skycoin-cx, and every program run from the bundle on both executors prints them |
| `benchmark-opcounts` | the operation counts in `suite.json` (needs `-tags cxopcount`) |
| `results-schema-roundtrip` | a document encodes and decodes unchanged, malformed ones are refused |
| `collector-accept-reject` | the collector stores a valid upload and refuses duplicates, bad and oversized ones |

## Known limits

A program cannot be interrupted. Back stops a `gfx` program at its next
`app.ShouldClose()`; a console program runs to the end first.

## Look

The whole UI, CX `app.Text` included, is JetBrains Mono at one size on a
dark background, plain text lines with no boxes. The font is under the SIL
Open Font License 1.1, see `cmd/cxplayground/font/OFL.txt`.
