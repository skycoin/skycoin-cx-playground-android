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

## Known limits

A program cannot be interrupted. Back stops a `gfx` program at its next
`app.ShouldClose()`; a console program runs to the end first.

## Look

The whole UI, CX `app.Text` included, is JetBrains Mono at one size on a
dark background, plain text lines with no boxes. The font is under the SIL
Open Font License 1.1, see `cmd/cxplayground/font/OFL.txt`.
