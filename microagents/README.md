# Micro agents in CX with deterministic replay

The stretch goal of the Android emulator rig spec (section 9, R13 to R16):
three micro agents ported to CX file by file, every input of each port
behind a wrapper that can record and replay, a memoizing model backend, and
tests that compare the port with the original.

| Agent | C and header lines | C lines the harness compiles | Port files (CX lines) | Tests passing | License |
|---|---|---|---|---|---|
| [manjunathshiva/femtoclaw](femtoclaw/README.md) | 8765 | 2629 | 84 (1998) | 53 of 53 | MIT |
| [memovai/mimiclaw](mimiclaw/README.md) | 9466 | 2393 | 90 (2162) | 40 of 40 | MIT |
| [tnm/zclaw](zclaw/README.md) | 10531 | 4240 | 98 (2379) | 166 of 166 | MIT |

The first column counts the `.c` and `.h` files under upstream `main/`. The
second counts the upstream `.c` files the host harness builds, which is the
core each port covers; the agent READMEs list what is left out.

## Layout

```
microagents/
  run-tests.sh          builds the tools into build/ and runs every test
  backendstore/         memoizing model backend, stub model and HTTP client
  portlayout/           checker for the R13 file layout
  testgen/              writes the generated test folders from tests.table
  testlib/              the script every generated test runs, and its recorder
  codingbench/          the nano agent coding benchmark: tasks, runner, runs
  femtoclaw/, mimiclaw/, zclaw/
    original/           unmodified upstream files
    before/             host harness for the original
    port/               the CX port, 000-....cx to nnn-main.cx
    record.sh           records a test
    tests.table         the per tool, pair and chain tests of the agent
    tests/<name>/       one folder per test
```

This folder is its own Go module, so its bbolt dependency stays out of the
app's vendor tree.

## Wrappers (R14)

Every file read, write and append, directory listing, model request, clock
read and inbound message of a port goes through a wrapper in `port/` chosen
at start with `++mode=live|record|replay`. Live does the operation. Record
does it and appends a record to the tape. Replay takes the result from the next tape
record and stops with exit code 3 when the port asks for something else: a
different kind of operation, another file, or a model request that is not
byte for byte the recorded one. A file write or append in replay is
checked against the tape instead of written. With `++apply=1` a replay also
applies them, which rebuilds the files a recording left, for the coding
benchmark. The ports use no random numbers. Each agent README lists the
records its port adds.

The tape is text, one record per line, fields separated by tabs, with `\n`,
`\t`, `\r` and `\\` escaped:

```
inbound	telegram	1001	What time is it?
file-read	/spiffs/config/SOUL.md	found	You are calm and brief.\n
clock	1795399339
net-request	http://127.0.0.1:18080/v1/messages	{"model":"claude-opus-4-5",...}
net-reply	200	{"id":"msg_stub_...",...}
file-append	/spiffs/sessions/tg_1001.jsonl	{"role":"user",...}\n
file-write	/spiffs/todo.md	- buy milk\n	ok
dir-list	/spiffs	ok	config/SOUL.md\nnotes.md\n
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
`[[status N]]` makes it answer with HTTP status N. `[[then]]` splits the
directives into steps: the stub answers the message with the tools of the
first step, the tool results with the tools of the next step, and so on,
and `{{result N}}` in a step becomes result N of the step before it, so a
later call can depend on an earlier one.

`backendstore post` is the HTTP client that the CX ports and the C
harnesses use, since the CX tcp package cannot read or write a connection,
and `backendstore head` prints the Date header of a HEAD request for the
mimiclaw clock. `backendstore listdir` lists the files below a folder as
the flat SPIFFS namespace, for the `dir-list` wrapper and the harness, and
`backendstore import` fills a store from the model records of a tape.

## Running the tests

```
SKYCOIN_CX_DIR=~/go/src/github.com/skycoin/skycoin-cx ./run-tests.sh
```

`CX=/path/to/cx` uses a built cx, and with neither variable the script runs
`go install github.com/skycoin/skycoin-cx/t11-cx@main`. The C harnesses need
a C compiler and libcjson. `TESTS='zclaw/tests/pair-*'` runs a subset of the
test folders.

| Test | What passes |
|---|---|
| port-layout | R13: every port file holds one function, type or constant group, and uses only names from files that sort before it |
| replay-determinism | two replays of the port from the tape print byte identical output, equal to the recorded output |
| before-after | the unmodified agent core, fed the inbound messages and clock reads of the tape (and for zclaw the uptimes), prints the same bytes as the port, and every one of its model requests is found in the store the port recorded |
| replay-detects-divergence | a tape with one byte changed (SOUL.md for femtoclaw and mimiclaw, an inbound message for zclaw) stops the replay at the first model request it changes |
| store-detects-divergence | the original with one byte of its input changed (USER.md, or an inbound message for zclaw) gets replay misses from the store |
| tool-*, pair-*, chain-* | generated from `tests.table`: before-after and replay-determinism in one, see below |
| go-test | the unit tests of backendstore, and testgen and codingbench build |
| generated-folders | every generated folder matches its table and is recorded |
| codingbench/scripted-stub | the replay of every benchmark tape rebuilds files the checks accept, and the original agent solves the same tasks |

Results on 2026-11-23 with cx at skycoin-cx main 74aab390, and the same 264
checks at the version the app pins, df4c9879, with the 244 generated tests
summed per kind:

```
PASS go-test: unit tests of backendstore, and testgen and codingbench build
PASS femtoclaw/generated-folders: femtoclaw: 48 generated folders match tests.table
PASS mimiclaw/generated-folders: mimiclaw: 35 generated folders match tests.table
PASS zclaw/generated-folders: zclaw: 161 generated folders match tests.table
PASS femtoclaw/before-after: before and after: 1376 identical bytes, every request of the original found in the store
PASS femtoclaw/port-layout: 84 files, one declaration each, every name declared before use
PASS femtoclaw/replay-detects-divergence: the replay stopped at the diverging request: FATAL net-request body of 8736 bytes differs from the 8736 recorded bytes
PASS femtoclaw/replay-determinism: two replays: 1045 identical bytes, equal to expected-output.txt
PASS femtoclaw/store-detects-divergence: the store refused 7 diverging requests and the original printed only errors
PASS mimiclaw/before-after: before and after: 1723 identical bytes, every request of the original found in the store
PASS mimiclaw/port-layout: 90 files, one declaration each, every name declared before use
PASS mimiclaw/replay-detects-divergence: the replay stopped at the diverging request: FATAL net-request body of 7277 bytes differs from the 7277 recorded bytes
PASS mimiclaw/replay-determinism: two replays: 802 identical bytes, equal to expected-output.txt
PASS mimiclaw/store-detects-divergence: the store refused 9 diverging requests and the original printed only errors
PASS zclaw/before-after: before and after: 7323 identical bytes, every request of the original found in the store
PASS zclaw/port-layout: 98 files, one declaration each, every name declared before use
PASS zclaw/replay-detects-divergence: the replay stopped at the diverging request: FATAL net-request body of 6416 bytes differs from the 6416 recorded bytes
PASS zclaw/replay-determinism: two replays: 1605 identical bytes, equal to expected-output.txt
PASS zclaw/store-detects-divergence: the store refused 4 diverging requests and the original printed 4 retry errors
PASS femtoclaw/tool-*: 6 of 6 generated tests, before and after plus two identical replays
PASS femtoclaw/pair-*: 36 of 36 generated tests, before and after plus two identical replays
PASS femtoclaw/chain-*: 6 of 6 generated tests, before and after plus two identical replays
PASS mimiclaw/tool-*: 5 of 5 generated tests, before and after plus two identical replays
PASS mimiclaw/pair-*: 25 of 25 generated tests, before and after plus two identical replays
PASS mimiclaw/chain-*: 5 of 5 generated tests, before and after plus two identical replays
PASS zclaw/tool-*: 12 of 12 generated tests, before and after plus two identical replays
PASS zclaw/pair-*: 144 of 144 generated tests, before and after plus two identical replays
PASS zclaw/chain-*: 5 of 5 generated tests, before and after plus two identical replays
PASS codingbench/scripted-stub: replay of every tape, femtoclaw solved 12 of 12 tasks. mimiclaw solved 12 of 12 tasks. zclaw is not run: it has no file tools.
all tests passed
```

The "before" side runs on the Linux host, not in QEMU. The core files
compile unmodified against the shims in `<agent>/before/shim`, and for
zclaw against its own upstream host mocks, so no ESP-IDF toolchain is
needed. Device behavior such as timing, the C buffer limits of femtoclaw
and mimiclaw, and SPIFFS listing order is not covered.

## Tests per tool and combinations

Each agent's `tests.table` lists the cases of every ported tool, the call a
pair test makes of each tool, and a few chains. One folder per test is
generated from it. After a change to the table,

```
CX=... BACKENDSTORE=build/backendstore testlib/record-generated.sh NAME $(go run ./testgen -agent NAME)
```

writes the folders whose inputs changed, removes the ones the table no
longer has, and records the new ones against the stub:

- `tool-NAME` calls one tool once per message, with representative input
  and the error cases: a missing file, a bad or climbing path, a missing
  field, a field of the wrong type, keys in capitals (cJSON matches keys
  ignoring ASCII case), and input that is not an object. A JSON tool input
  is always valid JSON, as the API sends it parsed, so wrong types and
  non-object input stand in for bad JSON.
- `pair-A-then-B` is one turn in which the model calls A and, after its
  result, B. There is one for every ordered pair of ported tools, the same
  tool twice included.
- `chain-NAME` is a longer turn: write, read, edit and list, a copy whose
  write carries the read result (`{{result 1}}`), a skill written in one
  turn and listed in the next system prompt, two tools per step, and a
  chain one step past the agent's tool round limit.

Every generated test runs `testlib/tool-test.sh`, which is a before and
after and a replay determinism test in one: two replays of the port print
identical bytes equal to `expected-output.txt`, and the unmodified original,
fed the tape's inbound messages and clock reads and started on the same
files, prints the same bytes while every model request it makes is found in
a store filled from the tape. The store is built from the tape at run time
(`backendstore import`), so the folders keep no bbolt file.
`testgen -check` fails when a folder differs from its table.



## Nano agent coding benchmark

`codingbench/tasks/<task>/` holds a coding task the agents solve with their
file tools: `prompt.txt`, the starting `files/` (they become `/spiffs`),
`check.sh`, which judges the files the agent leaves (for CX tasks by running
the program with cx and comparing its output), and `solution.json`, the
scripted solution. The tasks go from easy to multi-step:

| Task | What it takes |
|---|---|
| write-notes-file | write a one line text file |
| create-hello | create a CX program |
| fix-greeting-typo | one edit in a CX file |
| raise-the-limit | change a constant in a second file of a program |
| remove-debug-print | delete a line |
| create-fizzbuzz | create a program with a loop and branches |
| fix-sum-bug | read, then fix an off by one |
| rename-across-files | rename a variable in two files |
| add-function-and-call | read two files, add a function to one and call it from the other |
| copy-template-and-adapt | copy a file through the read result, then edit the copy |
| move-function-to-new-file | read, write a new file, cut the function, list |
| find-and-fix-compile-error | list, read three files, fix the one that does not compile |


The runner runs the CX port of each agent on each task through the
memoizing backend and writes a table of task, agent, solved, model turns,
tool calls, bytes sent and received, and wall time:

```
go build -o build/codingbench ./codingbench
build/codingbench -mode record -scripted -run codingbench/runs/scripted
build/codingbench -mode record -upstream https://api.anthropic.com/v1/messages -run codingbench/runs/NAME
build/codingbench -mode replay -run codingbench/runs/scripted
```

Record mode with `-upstream` forwards every model request to a real model,
with the key from `ANTHROPIC_API_KEY` (`-api-key-env` names another
variable), and keeps each tape under the run folder. Replay needs no model
or network: it replays each tape with `++apply=1`, judges the rebuilt files
again, and runs the unmodified original agent on the task against a store
filled from the tape, judging its files too. zclaw has no file tools and is
not run.

There is no model key on the machine that made this, so the only run is a
scripted baseline: `-scripted` records against the stub, with the steps of
each task's `solution.json` as tool directives after the prompt. It proves
the harness, the tools and the checks end to end. It says nothing about how
well a model codes, which needs a run with `-upstream` and a key.

The scripted baseline, recorded into `codingbench/runs/scripted-stub` with cx at
skycoin-cx main 74aab390:

| Task | Agent | Solved | Model turns | Tool calls | Bytes sent | Bytes received | Wall time | Note |
|---|---|---|---|---|---|---|---|---|
| add-function-and-call | femtoclaw | yes | 3 | 4 | 29703 | 1336 | 9.8 s |  |
| add-function-and-call | mimiclaw | yes | 3 | 4 | 25452 | 1336 | 7.4 s |  |
| copy-template-and-adapt | femtoclaw | yes | 4 | 3 | 38037 | 1226 | 9.7 s |  |
| copy-template-and-adapt | mimiclaw | yes | 4 | 3 | 32369 | 1226 | 7.4 s |  |
| create-fizzbuzz | femtoclaw | yes | 2 | 1 | 18976 | 877 | 3.5 s |  |
| create-fizzbuzz | mimiclaw | yes | 2 | 1 | 16142 | 877 | 2.9 s |  |
| create-hello | femtoclaw | yes | 2 | 1 | 18070 | 620 | 4.1 s |  |
| create-hello | mimiclaw | yes | 2 | 1 | 15236 | 620 | 3.7 s |  |
| find-and-fix-compile-error | femtoclaw | yes | 4 | 5 | 39109 | 1324 | 9.0 s |  |
| find-and-fix-compile-error | mimiclaw | yes | 4 | 5 | 33441 | 1324 | 15.9 s |  |
| fix-greeting-typo | femtoclaw | yes | 2 | 1 | 17822 | 578 | 2.6 s |  |
| fix-greeting-typo | mimiclaw | yes | 2 | 1 | 14988 | 578 | 1.4 s |  |
| fix-sum-bug | femtoclaw | yes | 3 | 2 | 27660 | 852 | 2.5 s |  |
| fix-sum-bug | mimiclaw | yes | 3 | 2 | 23409 | 852 | 3.9 s |  |
| move-function-to-new-file | femtoclaw | yes | 5 | 4 | 50533 | 1562 | 6.8 s |  |
| move-function-to-new-file | mimiclaw | yes | 5 | 4 | 43448 | 1562 | 3.9 s |  |
| raise-the-limit | femtoclaw | yes | 2 | 1 | 17989 | 599 | 2.2 s |  |
| raise-the-limit | mimiclaw | yes | 2 | 1 | 15155 | 599 | 1.6 s |  |
| remove-debug-print | femtoclaw | yes | 2 | 1 | 17932 | 608 | 2.0 s |  |
| remove-debug-print | mimiclaw | yes | 2 | 1 | 15098 | 608 | 1.4 s |  |
| rename-across-files | femtoclaw | yes | 3 | 3 | 29122 | 1086 | 3.3 s |  |
| rename-across-files | mimiclaw | yes | 3 | 3 | 24871 | 1086 | 2.4 s |  |
| write-notes-file | femtoclaw | yes | 2 | 1 | 17749 | 553 | 1.6 s |  |
| write-notes-file | mimiclaw | yes | 2 | 1 | 14915 | 553 | 1.7 s |  |

femtoclaw solved 12 of 12 tasks. mimiclaw solved 12 of 12 tasks. zclaw is not run: it has no file tools.


Its replay, which `run-tests.sh` runs, finds every file the port wrote
again and the original agent solving every task as well. Wall time is the
recording, with the stub and the backend store on the same host.

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
- The smallest i32 cannot be written as a literal: `var x i32 = -2147483648`
  fails with "invalid int literal: 2147483648", since the minus applies after
  2147483648 is read as an i32. `-2147483648L` for i64 works. The zclaw port
  writes `-2147483647 - 1`.
- `sys/os` cannot list a directory, so the `dir-list` wrapper runs
  `backendstore listdir` with `os.Run`.
