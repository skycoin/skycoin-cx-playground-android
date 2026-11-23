# Program distribution

This document describes how a CX program travels from the machine that
publishes it to a phone (and later an ESP32) that runs it. It covers the
signed manifest format (#14), the UDP operations that fetch a manifest and
its blocks (#15), the checks made before a program runs (#16), the loader,
lister and runner (#17), the DHT prototype (#18) and the hash, memory,
storage and serial test programs (#19 and skycoin-cx-forth #11). It ends
with the open decisions and a task breakdown down to tasks of about one pull
request each.

The terms of the project lead are used here. Where a standard term exists,
it is named next to the lead's term, and the two do not correspond exactly.

## Terms

This section defines the words the rest of the document uses.

- A *program* is one CX app: its source or serialized IR, its `cxapp.json`
  metadata file and any data files. It is what the app list shows today.
- A *block* is a piece of a file of at most the file's block size, 512
  bytes by default. The last block of a file can be shorter. A block is what
  BitTorrent calls a piece, but a block is much smaller.
- A *block ID* is the number of a block within one manifest. Block IDs start
  at 1 and count up across all files, in file order.
- The *linear packing* is the sequence of all blocks of a manifest, in block
  ID order, laid one after another with no padding. Because a short last
  block is not padded, the linear packing is also the concatenation of all
  files. Every file and every block is an offset and length range in it.
- A *manifest* is the signed description of a set of files: their hashes,
  lengths, blocks, and where each block sits in the linear packing.
- The *manifest address* is the SHA-256 hash that names a manifest. It is
  what a list of manifests holds, and what every request names. It is the
  content address (the standard term is content identifier, CID).
- A *signer* is the holder of the secp256k1 key that signs a manifest. The
  key is the same kind of key that Skycoin addresses and Skywire visors use.
- The *Merkle hash* is the root of a binary hash tree over block hashes.
  The standard term is Merkle root.
- A *manifest list* is a manifest whose files are other manifests. The
  server publishes one, so a client can learn what the server offers with
  the same operations it uses for programs.

## What exists now

This section records the starting point, so each change below says what it
replaces.

- `manifest/manifest.go` describes an app as JSON. Each file has a SHA-256
  hash and a list of SHA-256 chunk hashes, with a chunk size of 65536 bytes.
  The app hash is the SHA-256 of the encoded JSON.
- `store/store.go` defines `store.Source` with `Index`, `Manifest` and
  `Chunk`, and `store.FSSource` reads the bundle that `cxmanifest bundle`
  embeds in the APK. `store.List`, `store.Get` and `store.Load` verify every
  chunk and file hash.
- `cxrun` runs a program and gives it `app.NetDial`, `app.NetSend`,
  `app.NetRecv` and `app.NetClose` for TCP and UDP.
- `cmd/emulator-driver` runs every test folder under `tests/` on a headless
  Android emulator in the `cx-android-rig` container. `netecho` and
  `cmd/net-echo-server` run in that container, which the emulator reaches
  as 10.0.2.2.
- `microagents/backendstore` keeps records in bbolt.

Nothing is signed today, and nothing fetches a program over the network.

## Manifest format (#14)

This section defines the bytes of a manifest, how it is signed and how it
is verified. The format is binary, and the reasons are in decision D24.

### Layout

A manifest is a header followed by K arrays. Each array has an entry in the
header's array directory, which gives its kind, its entry count, where it
starts, its length and its SHA-256 hash. A client can therefore read and
verify the file list without reading the block arrays. All integers are
unsigned and big-endian.

The header:

| Offset | Size | Field |
|---|---|---|
| 0 | 4 | magic `CXPM` |
| 4 | 2 | format version, 1 |
| 6 | 2 | array count K |
| 8 | 8 | date, Unix seconds when the manifest was signed |
| 16 | 8 | manifest length, the byte length of the whole manifest |
| 24 | 8 | linear packing length, the sum of all file lengths |
| 32 | 32 | Merkle hash over every block hash, in block ID order |
| 64 | 48 × K | array directory |
| 64 + 48K | 33 | signer public key, compressed secp256k1 |
| 97 + 48K | 65 | signature |

Each array directory entry:

| Size | Field |
|---|---|
| 2 | array kind |
| 2 | reserved, 0 |
| 4 | entry count |
| 4 | offset of the array from the start of the manifest |
| 4 | byte length of the array |
| 32 | SHA-256 of the array bytes |

With the three array kinds below, the header is 306 bytes, which fits in the
first reply packet.

The arrays follow the header in directory order. Version 1 defines three
kinds, and a reader skips a kind it does not know (its hash still counts,
because the hash is signed).

1. *File list* (kind 1). One entry per file, in path order:

   | Size | Field |
   |---|---|
   | 32 | SHA-256 of the whole file |
   | 8 | file length |
   | 8 | offset of the file in the linear packing |
   | 4 | block size of this file, 512 by default |
   | 4 | first block ID of the file |
   | 4 | block count of the file |
   | 32 | Merkle hash over the file's block hashes |
   | 2 | path length |
   | n | path, UTF-8, slash separated, no `..` |

   The file's range in the linear packing is its offset and its file length.
   Its blocks are the block IDs from the first block ID for block count IDs.
   This is how a file entry carries its block hash list and block size: the
   hashes themselves sit in the block list, at the end of the manifest, as
   #14 asks.

2. *Block list* (kind 2). One entry of 48 bytes per block, in ID order:

   | Size | Field |
   |---|---|
   | 4 | block ID |
   | 32 | SHA-256 of the block |
   | 4 | block length |
   | 8 | offset of the block within its file |

3. *Block packing map* (kind 3). One entry of 16 bytes per block, in ID
   order, mapping the block ID to its range in the linear packing:

   | Size | Field |
   |---|---|
   | 4 | block ID |
   | 8 | offset in the linear packing |
   | 4 | block length |

The block IDs, lengths and offsets are redundant with the file list, and the
reader checks that they agree. The lead asked for them to be explicit, and
they let a client that only holds the block arrays serve or fetch a range.

The manifest length is 306 bytes plus the array lengths. A program of 64 KB
has 128 blocks, so its block list is 6144 bytes and its packing map 2048
bytes, about 8.5 KB in all.

### Merkle hash

The Merkle hash lets a client check a complete set of block hashes against
one signed value.

- A leaf is a block hash, the plain SHA-256 of the block bytes.
- An inner node is SHA-256 of the byte 0x01, the left child and the right
  child.
- When a level has an odd number of nodes, the last node moves up unchanged.
  It is not paired with itself, which avoids the duplicate-leaf ambiguity of
  the Bitcoin tree (CVE-2012-2459).
- The root of an empty list is the SHA-256 of nothing.

The header holds the root over all blocks of the manifest, and every file
entry holds the root over its own blocks, so one file can be checked
without the others.

### Address and signature

The signature lets a client reject a manifest from an unknown signer after
the first packet.

- The *signed part* is the header from offset 0 through the signer public
  key, that is every header byte except the signature.
- The manifest address is the SHA-256 of the signed part.
- The signature is the Skycoin `cipher.SignHash` of the address with the
  signer's secret key: 65 bytes, a compact secp256k1 signature with a
  recovery byte.
- Because the header holds the hash of every array, the address covers every
  byte of the manifest except the signature.

The address cannot be stored inside the manifest, because a hash cannot hold
itself. The header holds the manifest length, the date and the Merkle hash,
and the address is what names the manifest from outside.

### How the manifest itself is chunked

A manifest is read like a file, by offset and length, in pieces of at most
one reply packet. The pieces are checked in this order:

1. The first piece holds the header. The client checks the magic, version
   and lengths, computes the address and compares it with the address it
   asked for, and verifies the signature against the signer key. When the
   signer is not trusted, the client stops here.
2. Each array is checked against its directory hash as soon as all its bytes
   are in. The file list usually arrives in the first or second piece.
3. When the block list is complete, the client recomputes the Merkle hash
   and every file's Merkle hash and compares them with the signed values.

### Verification of the files

A client checks every block as it arrives and every file once it is whole.

- A block is accepted only when its SHA-256 matches its block list entry and
  its length matches.
- A file is accepted only when all its blocks are in, its length matches and
  the SHA-256 of the whole file matches its file list entry.
- The program is accepted only when every file is accepted.

### Relation to the JSON manifest

The JSON manifest stays for the apps embedded in the APK, and the binary
format is used for programs fetched over the network. The loader of #17
moves the embedded bundle to the binary format in a later pull request.
Decision D25 asks whether to replace the JSON manifest at once instead.

The program metadata (name, kind, form, entry, arguments) stays in
`cxapp.json`, which is an ordinary file of the binary manifest. A client
rebuilds a `manifest.Manifest` from the downloaded files with
`manifest.Build`, and runs it with `apprun.Run` as today.

## UDP operations (#15)

This section defines the packets that fetch a manifest list, a manifest and
its blocks, and how a client recovers from loss. Every packet is at most
1200 bytes, and a 1024-byte variant is supported by configuration
(decision D26).

### Common header

Every packet starts with six bytes:

| Size | Field |
|---|---|
| 1 | protocol version, 1 |
| 1 | operation code, with the high bit set in a reply |
| 4 | request ID, chosen by the client and echoed in the reply |

A reply carries the request's operation code with 0x80 added, or 0xFF for
an error.

### Operations

The sizes below are for a 1200-byte packet, with the 1024-byte values in
brackets.

1. *List manifests* (0x01). The request is the header only. The reply holds
   the 32-byte address and the 4-byte length of the server's manifest list.
   The client then fetches the list like any manifest. Each file of the list
   is a program manifest, and its path is the program name.
2. *Get manifest* (0x02). The request holds the address, a 4-byte offset and
   a 2-byte length. The reply holds the address, the offset, the 4-byte total
   manifest length and the bytes, at most 1154 (978) of them.
3. *Get block list* (0x03). The request holds the address, a 4-byte first
   block ID and a 2-byte count. The reply holds the address, the first ID,
   the count and the 48-byte block list entries (hash, length and offset), at
   most 24 (20) of them. This is the "get hash list with length and offset"
   operation.
4. *Get blocks* (0x04). The request holds the address, an 8-byte offset and
   a 2-byte length in the linear packing. The reply holds the address, the
   offset, the length and the bytes, at most 1152 (976) of them. Any range
   can be asked for. A client asks for whole blocks, two 512-byte blocks per
   packet (one at 1024), so each reply can be verified on arrival.
5. *Error* (0xFF, reply only). It holds a 1-byte code, a 1-byte text length
   and a short text. The codes are 1 unknown manifest, 2 range outside the
   manifest, 3 malformed request and 4 unsupported version.

A server never sends more than the packet size. It shortens a reply that
would not fit, and the client asks again for what is missing.

### Loss, retries and order

UDP packets can be lost, repeated or reordered, so the client handles all
three.

- The client keeps up to 16 requests in flight and matches each reply by
  its request ID. A reply to an unknown or finished request is dropped.
- A request with no reply after 250 ms is sent again, with the wait doubled
  each time up to 2 s, and the fetch fails after 8 tries of one request.
- Every request is idempotent, so a repeated reply does no harm.
- A block that fails its hash check is asked for again, and the fetch fails
  when the same block fails three times, because then the server holds bad
  data.

### Incremental verification

A client verifies as it goes, so it never holds unverified data for long.

1. List manifests, then get manifest offset 0 of the list: check the address
   and the signature from the first packet.
2. Fetch the rest of the list's arrays and its blocks, which are the program
   manifests, verifying each piece.
3. For the chosen program, get manifest offset 0, check its address and
   signature, then fetch and check its arrays.
4. Get blocks two at a time from the linear packing, checking each block on
   arrival and each file when it is whole.

## Validation before running (#16)

This section describes the check made on a downloaded program before it
runs, and how the result is shown. Validation is not part of this batch.

The check reads the program without running it:

- For a serialized IR program, it walks every function's expressions. Each
  `OP_LIBRARY_CALL` names a library number and a function number from
  `constants.LibraryTable` and `constants.LibraryFunctionLifecycleTable` of
  skycoin-cx (#492). The libraries are file (1), net (2) and process (3).
- For a source program, it compiles the program with the playground's
  compiler and walks the result the same way.
- Network use is reported, and UDP use is reported apart from TCP. In the
  playground today, `app.NetDial` with the constant `"udp"` is UDP. A
  network name that is not a constant is reported as unknown.
- Imports are listed. An import on a blocked list (for example `os` or
  `process` for a program that came from the network) fails the check.
- Each opcode is looked up in an opcode set: the core set, the app set
  (opcodes from 4096), and the system library set. An opcode outside the
  sets allowed for the program fails the check.
- Minor IR checks run: every opcode number is known, every function call
  names a function that exists, and argument counts match.

The result is a short monospaced report shown before the run, one line per
check, for example `file system no`, `networking yes, udp yes`, `process
no`, `imports app string time`, `blocked imports none`, `sandbox
violations none`. A failing program can be inspected but not run.

## Loader, lister and runner (#17)

This section describes the screen and the storage layer that hold programs
on the phone, and the path to the ESP32. It is not part of this batch.

### Screen

The screen stays monospaced at one text size, as today. Each row shows the
program number, the name, whether it is cached, held in RAM or remote, and
its length. Selecting a row offers download (or cache), validate and run.

### Storage layout

The SD card (or the app's files directory on the phone) holds:

- `programs/list.bin`, the last verified manifest list;
- `programs/m/<address>.bin`, one verified manifest per program;
- `programs/p/<address>.pack`, the program's linear packing, written block
  by block as blocks are verified, so a stopped download resumes;
- `programs/index.txt`, one line per program with its address, name, state
  and length, which the lister reads without opening any manifest.

The lister reads `index.txt` with one sequential read and no allocation per
line beyond the row. The loader reads the manifest and the packing into RAM.
The unloader drops them. The runner runs what the loader holds.

### Memory budget

A phone has room for every program. An ESP32 has about 300 KB of free RAM,
so there the loader keeps at most one program in RAM, reads blocks from the
SD card as needed, and the manifest's block arrays are read in pieces rather
than held whole.

### ESP32 path

On the ESP32, the runner is cx-forth (skycoin-cx-forth), which runs a CX IR
subset as Forth, or a `cx build` core when it fits. The program is stored as
the serialized IR or the Forth form in the linear packing. The fetcher is the
same packet code, built with TinyGo.

### Suspend and resume (later)

Saving the VM state (stacks, memory, registers, open handles) to the SD
card allows pause, suspend, resume and switching programs. cx-forth has
fixed-size stacks and memory, so its state is a fixed set of byte ranges
plus the program address, which makes it the first target. Each save and
load is timed and reported.

## DHT prototype (#18)

This section describes the prototype network that spreads programs between
nodes. It is not part of this batch.

### Packets

The DHT reuses the common packet header and adds four operations:

- *Have* (0x10): a node announces that it holds data with a hash and a
  length. For a program, the hash is the manifest address.
- *Subscribe* (0x11): a node asks a peer to send it have packets.
- *Get peers* (0x12): a node asks for peers, and the reply lists peers with
  their address and group, within one packet.
- The download uses the operations of #15 against any node that sent a
  have.

### Flood replication

When a node receives a have for a hash it does not hold, it downloads and
verifies the data, then sends its own have to all its subscribers. A node
sends a have for a hash at most once, which ends the flood. Only manifests
from trusted signers are fetched and forwarded.

### Peer lists

Each peer record holds an address, a group and a sharing class:

1. private, not shared;
2. private own infrastructure, shared only with the owner's own nodes;
3. LAN group, shared only within that LAN group;
4. user-selected shareable;
5. public shareable.

A get-peers reply includes a peer only when its class allows sharing with
the asking node.

### Address scopes and groups

An address scope says where an address is reachable: LAN, internal (a
private network such as one inside a country) or global. A group has a
name, such as `lan1`, `china1`, `china-internal` or `global`, an owner, an
operator and a security group. A node only hands out an address to peers in
a group that can reach its scope.

The peer list is managed with a command-line tool first.

## Test programs (#19 and skycoin-cx-forth #11)

This section describes the hash, memory, storage and serial test programs
and how their results are reported. They are not part of this batch.

### Report

Every test prints one line with its name, `pass true` or `pass false`,
blocks per second, KB/s, latency in microseconds, and, for serial tests, the
read and write speeds. Test names are lexicographic and hierarchical:

- `HashSha256Known`, `HashMerkleKnown`;
- `MemoryPatternWalkingOnes`, `MemoryPatternWalkingZeros`,
  `MemoryPatternAddress`, `MemoryPatternRandomSeeded`;
- `StorageBlockRead512`, `StorageBlockWrite512`, `StorageBlockRead4096`,
  `StorageBlockWrite4096`, `StorageFileSystemRead`, `StorageFileSystemWrite`;
- `SerialUartRead`, `SerialUartWrite`, `SerialSpiRead`, `SerialSpiWrite`.

### Phone

The hash and memory program is CX and runs in the emulator rig with one
test folder per test. On the phone, an app cannot open the block device, so
method A is the file system (one file per block size) and method B is
direct block access to one preallocated file with a sync after every write
(decision D29). The phone has no UART or SPI, so the serial tests report
that they are skipped.

### ESP32

On the ESP32 (skycoin-cx-forth #11), method A is the FAT file system on the
SD card and method B is raw sector reads and writes over SPI. UART and SPI
speeds are measured with a loopback. The tests run in Espressif QEMU first
(`scripts/qemu-esp32.sh`), where SD and serial speeds are not meaningful,
and then on a board.

## Decisions

This section lists the choices the lead left open. The implementation uses
each recommended option, and each one is a constant or a small function so
it can be changed.

D22 ProgramManifestSignatureSchemeDecision
- What exists now: nothing in the playground is signed. Skycoin and Skywire
  use secp256k1 keys through `github.com/skycoin/skycoin/src/cipher`, which
  the playground already vendors for skycoin-cx.
- Question: which signature scheme signs a manifest?
- Option a: secp256k1 with the Skycoin cipher package. Public keys are 33
  bytes and signatures 65 bytes, and a signer can use the same key as their
  Skycoin address or Skywire visor.
- Option b: ed25519 from the Go standard library. Public keys are 32 bytes
  and signatures 64 bytes, verification is faster, and the keys are a new
  kind unrelated to Skycoin and Skywire.
- Recommendation: option a, because one key identifies the same person or
  node across Skycoin, Skywire and program distribution, and the package is
  already in the build.

D23 ProgramBlockHashFunctionDecision
- What exists now: `manifest.Sum` uses SHA-256 for files and chunks.
- Question: which hash names blocks, files, arrays and manifests?
- Option a: SHA-256, as today. It is in the Go standard library and has
  hardware support on phones and on the ESP32.
- Option b: BLAKE3. It is faster in software and has a tree mode of its own,
  but needs a new dependency and has no ESP32 hardware support.
- Recommendation: option a, because the ESP32 has SHA-256 hardware, the
  existing code uses it, and a 512-byte block costs little to hash either
  way.

D24 ProgramManifestEncodingDecision
- What exists now: `manifest.Manifest` is JSON, encoded with
  `json.MarshalIndent`.
- Question: is the new manifest binary or JSON?
- Option a: binary, with the header, array directory and arrays of this
  document. It has fixed offsets, can be read by array without a parser, and
  is about a third of the JSON size with raw hashes.
- Option b: JSON with hex hashes. It is readable by eye, but a reader must
  parse all of it, hashes double in size, and the ESP32 needs a JSON parser.
- Recommendation: option a, because the manifest is read a packet at a time
  and on the ESP32, and `cmd/programblocks show` prints it as text.

D25 ProgramManifestReplacesJsonManifestDecision
- What exists now: `cxmanifest bundle` writes JSON manifests into
  `cmd/cxplayground/bundle`, and `store.FSSource` reads them.
- Question: does the binary manifest replace the JSON manifest now?
- Option a: keep the JSON manifest for the embedded apps now, use the binary
  manifest for programs over the network, and move the bundle to the binary
  manifest in the loader work of #17.
- Option b: replace the JSON manifest everywhere in this batch, including
  the bundle, `store` and `cxmanifest`.
- Recommendation: option a, because it keeps each pull request small and the
  bundle tests unchanged while the network format settles.

D26 ProgramPacketSizeDecision
- What exists now: there is no program packet.
- Question: what is the largest packet?
- Option a: 1200 bytes, which passes the 1280-byte IPv6 minimum MTU with
  room for headers and carries two 512-byte blocks per packet.
- Option b: 1024 bytes, which carries one 512-byte block per packet, so a
  fetch takes about twice the packets.
- Recommendation: option a as the default, with 1024 as a setting
  (`-packet 1024`), because two blocks per packet halve the round trips and
  1200 bytes still fits every path that carries IPv6.

D27 ProgramTrustedSignerDecision
- What exists now: nothing is signed, so nothing is trusted.
- Question: which signers does the app accept?
- Option a: a list of trusted public keys kept in a file next to the
  programs (and given as an argument to the fetch), and a manifest from any
  other key is rejected after its first packet.
- Option b: trust on first use, where the first key seen from a server is
  kept and later keys are rejected.
- Option c: accept every signer and show the key.
- Recommendation: option a, because a program can reach the network and
  files, and only a pinned key keeps a stranger's program from running.

D28 ProgramFetchClientPlacementDecision
- What exists now: CX programs reach UDP through `app.NetDial` and friends,
  but no CX program can start another program, and the playground's CX has
  no SHA-256 or secp256k1 calls.
- Question: does the fetch and verify code run in CX or in the Go side of
  the app?
- Option a: in Go, in the `programblocks` package, called by the app. A CX
  program can trigger it later through an app call.
- Option b: in CX over `app.Net*`, after adding hash and signature calls and
  a way to hand the result to the runner.
- Recommendation: option a, because the same Go code serves the host
  command, the tests, the phone and (with TinyGo) the ESP32, and it does not
  wait for new CX calls.

D29 StorageDirectBlockMethodOnPhoneDecision
- What exists now: Android apps cannot open the SD card block device without
  root.
- Question: what is method B, direct block access, on the phone?
- Option a: reads and writes of 512 to 4096-byte blocks at fixed offsets in
  one preallocated file, with a sync after each write.
- Option b: skip method B on the phone and run it only on the ESP32.
- Recommendation: option a, because it measures the cost of the file
  system's metadata path against plain block writes, and it is the same test
  code on both targets.

## Task breakdown

This section breaks #14 to #19 and skycoin-cx-forth #11 down into leaf tasks
of about one pull request each. Every leaf has a hierarchical name, its
dependencies and its test.

- ProgramDistribution (all issues)
  - ProgramDistributionDesignDocument: this document. Depends on nothing.
    Test: review by the lead.
  - ProgramManifest (#14)
    - ProgramManifestBlockSplit: split files into 512-byte blocks with IDs
      and offsets. Depends on the design. Test: round trip.
    - ProgramManifestMerkleRoot: the tree of this document. Depends on
      BlockSplit. Test: known roots, odd counts.
    - ProgramManifestBinaryEncode: header, directory and three arrays,
      encode and decode with all cross checks. Depends on BlockSplit and
      MerkleRoot. Test: truncated manifest rejected.
    - ProgramManifestSignature: address, sign and verify with the Skycoin
      cipher. Depends on BinaryEncode. Test: wrong signature detected.
    - ProgramManifestAssemble: accept blocks in any order, verify each block
      and file. Depends on BinaryEncode. Test: corrupted block detected.
    - ProgramManifestList: a manifest of manifests. Depends on Signature.
      Test: round trip through the server.
    - ProgramManifestBundleMigration: move the embedded bundle to the binary
      manifest (D25 option a, later). Depends on LoaderStore. Test: make
      check-bundle and the emulator suite.
  - ProgramTransport (#15)
    - ProgramTransportPacketCodec: encode and decode every packet, with the
      size limit. Depends on BinaryEncode. Test: packet sizes.
    - ProgramTransportServer: serve a directory of programs. Depends on
      PacketCodec and ManifestList. Test: round trip.
    - ProgramTransportClient: window, retries and incremental verification.
      Depends on PacketCodec and Assemble. Test: loss tolerance.
    - ProgramTransportCommand: `cmd/programblocks serve`, `fetch` and
      `show`. Depends on Server and Client. Test: round trip.
    - ProgramTransportEmulatorFetch: the app fetches, verifies and runs a
      program from the rig. Depends on Client and Command. Test:
      programs-fetch-over-udp-in-emulator.
  - ProgramValidation (#16)
    - ProgramValidationLibraryCalls: list library calls from the IR.
      Depends on skycoin-cx #492. Test: one program per library.
    - ProgramValidationNetworkUdp: report network and UDP use. Depends on
      LibraryCalls. Test: the UDP echo app reports udp yes.
    - ProgramValidationImports: list imports and fail blocked ones. Test: a
      program importing a blocked package fails.
    - ProgramValidationOpcodeSets: check opcodes against allowed sets. Test:
      an app opcode in a core-only program fails.
    - ProgramValidationReportScreen: show the report before the run. Depends
      on the four checks. Test: emulator test reading the report lines.
  - ProgramLoader (#17)
    - ProgramLoaderStore: the SD card layout and resumable packing. Depends
      on Assemble. Test: resume after a stopped download.
    - ProgramLoaderLister: read `index.txt` with low overhead. Depends on
      Store. Test: list of N programs, allocation count.
    - ProgramLoaderMemory: load, keep and clear a program in RAM. Depends on
      Store. Test: load, unload, reload.
    - ProgramLoaderScreen: the monospaced list with download, validate and
      run. Depends on Lister, Memory and ValidationReportScreen. Test:
      emulator test tapping a remote program.
    - ProgramLoaderEsp32Design: the ESP32 loader on cx-forth. Depends on
      Store. Test: review.
    - ProgramLoaderSuspendResume: save and load VM state (later). Depends on
      Memory. Test: suspend, resume, same output, timings.
  - ProgramNetwork (#18)
    - ProgramNetworkPacketHaveSubscribe: have and subscribe packets. Depends
      on PacketCodec. Test: codec sizes.
    - ProgramNetworkFloodReplication: download on have and forward once.
      Depends on HaveSubscribe and Client. Test: three nodes in one process.
    - ProgramNetworkPeerGetPeers: the get-peers packet. Depends on PacketCodec.
      Test: reply fits one packet.
    - ProgramNetworkPeerListClasses: the five sharing classes. Depends on
      GetPeers. Test: a private peer is never shared.
    - ProgramNetworkPeerGroupsScopes: groups and address scopes. Depends on
      ListClasses. Test: a LAN address is not given to a global peer.
    - ProgramNetworkPeerCommand: command-line peer list management. Depends
      on GroupsScopes. Test: round trip of the list file.
    - ProgramNetworkEmulatorFlood: the phone receives a program by flood.
      Depends on FloodReplication. Test: emulator test.
  - ProgramTestSuite (#19, skycoin-cx-forth #11)
    - ProgramTestSuiteReportFormat: the result line. Test: parse round trip.
    - ProgramTestSuiteHashKnown: HashSha256Known and HashMerkleKnown in CX.
      Test: emulator folder per test.
    - ProgramTestSuiteMemoryPatterns: the four memory patterns in CX. Test:
      emulator folder per test.
    - ProgramTestSuiteStorageFileSystem: method A at 512 and 4096. Test:
      emulator folder per test.
    - ProgramTestSuiteStorageDirectBlock: method B (D29). Test: emulator
      folder per test.
    - ProgramTestSuiteSerialSkipOnPhone: serial tests report skipped on the
      phone. Test: emulator folder.
    - ProgramTestSuiteEsp32Storage: SD over FAT and raw SPI sectors
      (forth #11). Test: QEMU, then a board.
    - ProgramTestSuiteEsp32Serial: UART and SPI loopback speeds (forth #11).
      Test: a board.
    - ProgramTestSuiteEsp32MemoryHash: memory and hash tests in cx-forth
      (forth #11). Test: QEMU.
