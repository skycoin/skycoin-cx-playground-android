# Micro agents in CX with deterministic replay

The stretch goal of the Android emulator rig spec (section 9, R13 to R16):
three micro agents ported to CX file by file, every input of each port
behind a wrapper that can record and replay, a memoizing model backend, and
tests that compare the port with the original.

| Agent | C and header lines | C lines the harness compiles | Port files (CX lines) | Tests passing | License |
|---|---|---|---|---|---|
| [manjunathshiva/femtoclaw](femtoclaw/README.md) | 8765 | 2568 | 73 (1638) | 5 of 5 | MIT |
| [memovai/mimiclaw](mimiclaw/README.md) | 9466 | 2393 | 80 (1838) | 5 of 5 | MIT |
| [tnm/zclaw](zclaw/README.md) | 10531 | 3526 | 90 (2133) | 5 of 5 | MIT |

The first column counts the `.c` and `.h` files under upstream `main/`. The
second counts the upstream `.c` files the host harness builds, which is the
core each port covers; the agent READMEs list what is left out.

## Layout

```
microagents/
  run-tests.sh          builds the tools into build/ and runs every test
  backendstore/         memoizing model backend, stub model and HTTP client
  portlayout/           checker for the R13 file layout
  femtoclaw/, mimiclaw/, zclaw/
    original/           unmodified upstream files
    before/             host harness for the original
    port/               the CX port, 000-....cx to nnn-main.cx
    record.sh           records a test
    tests/<name>/       one folder per test
```

This folder is its own Go module, so its bbolt dependency stays out of the
app's vendor tree.

## Wrappers (R14)

Every file read and append, model request, clock read and inbound message of
a port goes through a wrapper in `port/` chosen at start with
`++mode=live|record|replay`. Live does the operation. Record does it and
appends a record to the tape. Replay takes the result from the next tape
record and stops with exit code 3 when the port asks for something else: a
different kind of operation, another file, or a model request that is not
byte for byte the recorded one. A file append in replay is checked against
the tape instead of written. The ports use no random numbers. Each agent
README lists the records its port adds.

The tape is text, one record per line, fields separated by tabs, with `\n`,
`\t`, `\r` and `\\` escaped:

```
inbound	telegram	1001	What time is it?
file-read	/spiffs/config/SOUL.md	found	You are calm and brief.\n
clock	1795399339
net-request	http://127.0.0.1:18080/v1/messages	{"model":"claude-opus-4-5",...}
net-reply	200	{"id":"msg_stub_...",...}
file-append	/spiffs/sessions/tg_1001.jsonl	{"role":"user",...}\n
inbound-end
```

## Memoizing backend (R15)

`backendstore serve -store F -mode record|replay` serves the model API over
HTTP. The key is the hex SHA-256 of the exact request body and the value is
the exact response body, with its status, in a bbolt file. Record forwards a
miss to `-upstream` and stores the reply whatever its status, so a replay
fails where the recording failed. Replay answers a miss with 502,
logs it, and exits non-zero when stopped. `backendstore stub` is a
deterministic stand in for the model whose reply depends only on the request
bytes. It asks for `get_current_time` when the message mentions the time and
for `read_file` when it names a `/spiffs/` path. A test can also steer it:
`[[tool NAME JSON]]` in the last message makes it call that tool, and
`[[status N]]` makes it answer with HTTP status N. `backendstore post` is the
HTTP client that the CX ports and the C harnesses use, since the CX tcp
package cannot read or write a connection, and `backendstore head` prints the
Date header of a HEAD request for the mimiclaw clock.

## Running the tests

```
SKYCOIN_CX_DIR=~/go/src/github.com/skycoin/skycoin-cx ./run-tests.sh
```

`CX=/path/to/cx` uses a built cx, and with neither variable the script runs
`go install github.com/skycoin/skycoin-cx/t11-cx@main`. The C harnesses need
a C compiler and libcjson.

| Test | What passes |
|---|---|
| port-layout | R13: every port file holds one function, type or constant group, and uses only names from files that sort before it |
| replay-determinism | two replays of the port from the tape print byte identical output, equal to the recorded output |
| before-after | the unmodified agent core, fed the inbound messages and clock reads of the tape (and for zclaw the uptimes), prints the same bytes as the port, and every one of its model requests is found in the store the port recorded |
| replay-detects-divergence | a tape with one byte changed (SOUL.md for femtoclaw and mimiclaw, an inbound message for zclaw) stops the replay at the first model request it changes |
| store-detects-divergence | the original with one byte of its input changed (USER.md, or an inbound message for zclaw) gets replay misses from the store |

Results on 2026-11-23 with cx at skycoin-cx main 74aab390 and at the
version the app pins, df4c9879:

```
PASS femtoclaw/before-after: before and after: 1376 identical bytes, every request of the original found in the store
PASS femtoclaw/port-layout: 73 files, one declaration each, every name declared before use
PASS femtoclaw/replay-detects-divergence: the replay stopped at the diverging request: FATAL net-request body of 8736 bytes differs from the 8736 recorded bytes
PASS femtoclaw/replay-determinism: two replays: 1045 identical bytes, equal to expected-output.txt
PASS femtoclaw/store-detects-divergence: the store refused 7 diverging requests and the original printed only errors
PASS mimiclaw/before-after: before and after: 1723 identical bytes, every request of the original found in the store
PASS mimiclaw/port-layout: 80 files, one declaration each, every name declared before use
PASS mimiclaw/replay-detects-divergence: the replay stopped at the diverging request: FATAL net-request body of 7277 bytes differs from the 7277 recorded bytes
PASS mimiclaw/replay-determinism: two replays: 802 identical bytes, equal to expected-output.txt
PASS mimiclaw/store-detects-divergence: the store refused 9 diverging requests and the original printed only errors
PASS zclaw/before-after: before and after: 7323 identical bytes, every request of the original found in the store
PASS zclaw/port-layout: 90 files, one declaration each, every name declared before use
PASS zclaw/replay-detects-divergence: the replay stopped at the diverging request: FATAL net-request body of 6416 bytes differs from the 6416 recorded bytes
PASS zclaw/replay-determinism: two replays: 1605 identical bytes, equal to expected-output.txt
PASS zclaw/store-detects-divergence: the store refused 4 diverging requests and the original printed 4 retry errors
all tests passed
```

The "before" side runs on the Linux host, not in QEMU. The core files
compile unmodified against the shims in `<agent>/before/shim`, and for
zclaw against its own upstream host mocks, so no ESP-IDF toolchain is
needed. Device behavior such as timing, the C buffer limits of femtoclaw
and mimiclaw, and SPIFFS listing order is not covered.

## Recording

```
CX=... BACKENDSTORE=build/backendstore femtoclaw/record.sh femtoclaw/tests/before-after
```

Each agent has its own `record.sh`. This records against the stub. For a
real model set `UPSTREAM=https://api.anthropic.com/v1/messages` and
`ANTHROPIC_API_KEY`. Recording listens on 127.0.0.1:18080, since femtoclaw
puts that URL in its tape records. The mimiclaw recording also needs
network access for the HEAD request of `get_current_time`. The tests only
replay, so they need neither the stub nor a network.

## CX issues met

- `printf` and `sprintf` expand backslash escapes inside `%s` arguments, so
  `sprintf("%s", "\\n")` gives a newline. The port builds text with
  `joinStrings` and prints with `string.PrintLine`.
- String literals with `\x41`, `\u0041` or `\U00000041` fail to compile with
  "invalid character 'x' in hexadecimal escape". Octal escapes work, so the
  byte table uses `"\101"`.
- There is no conversion between `str` and `[]ui8`, and `os.WriteStr` adds a
  length prefix, so raw file writes go through a 256 entry byte table and
  `os.WriteUI8Slice`.
- `sys/tcp` can dial, listen and accept but not read or write, so model
  requests go through `os.Run` of `backendstore post`.
- `s.flag && f()`, with `flag` a bool struct field and `f` a call, is
  false when both are true, and with `s` a slice element it stops with
  CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE. `s.flag && true`, `f() && s.flag`
  and `s.flag || f()` work. The zclaw port reads the field into a local
  first.
