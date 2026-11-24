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

This part defines the bytes of a manifest in seven sections: the encoding
conventions, the header, the arrays, the linear packing, the chunking of the
manifest, the Merkle tree and the text form. The UDP packets follow in their
own part. The binary form is the canonical form, which means the address and
the signature are computed over its bytes. Decision D30 asks whether the text
form should be canonical instead.

A manifest is laid out in this order:

1. the header, which ends with the signer public key and the signature;
2. the file list array;
3. the block list array;
4. the block packing map array.

The header holds an array directory with one entry per array, so a client
can read and verify the file list without reading the block arrays.

### Binary encoding conventions

This section fixes how every field is written, so the other sections only
name fields and types.

- Every integer is unsigned and big-endian (network byte order). The types
  are `u16` (2 bytes), `u32` (4 bytes) and `u64` (8 bytes).
- A `hash` is a SHA-256 digest of 32 raw bytes (decision D23).
- A `pubkey` is a compressed secp256k1 public key of 33 bytes, and a `sig`
  is a compact secp256k1 signature of 65 bytes with its recovery byte
  (decision D22). These are the formats of the Skycoin `cipher` package.
- A `path` is a `u16` byte length followed by that many bytes of UTF-8. A
  path is slash separated, relative, at most 255 bytes, and has no `.` or
  `..` element.
- An array is a sequence of entries with no separator. Its entry count,
  byte offset, byte length and hash are in the array directory of the
  header, not in front of the array, so the count of every array is known
  from the first packet.
- There is no padding and no alignment anywhere.

### Manifest header

This section defines the header, which identifies the manifest, commits to
every array and carries the signature.

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 0 | 4 | bytes | magic, the ASCII text `CXPM` |
| 4 | 2 | u16 | format version, 1 |
| 6 | 2 | u16 | array count K, 3 in version 1 |
| 8 | 8 | u64 | date, Unix seconds when the manifest was built |
| 16 | 8 | u64 | manifest length, the byte length of the whole manifest |
| 24 | 8 | u64 | linear packing length, the sum of all file lengths |
| 32 | 32 | hash | Merkle hash over every block hash, in block ID order |
| 64 | 48 × K | entries | array directory, one entry per array, in array order |
| 64 + 48K | 33 | pubkey | signer public key |
| 97 + 48K | 65 | sig | signature |

Each array directory entry:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 0 | 2 | u16 | array kind: 1 file list, 2 block list, 3 block packing map |
| 2 | 2 | u16 | reserved, 0 |
| 4 | 4 | u32 | entry count |
| 8 | 4 | u32 | offset of the array from the start of the manifest |
| 12 | 4 | u32 | byte length of the array |
| 16 | 32 | hash | SHA-256 of the array bytes |

With K = 3, the header is 306 bytes. The arrays follow without gaps, so the
first array starts at the end of the header, each next array starts where the
previous one ends, and the last one ends at the manifest length.

What is signed:

- The *signed part* is the header from offset 0 up to the end of the signer
  public key, which is every header byte except the signature.
- The *manifest address* is the SHA-256 of the signed part.
- The signature is `cipher.SignHash(address, secret key)`, and a reader
  checks it with `cipher.VerifyPubKeySignedHash(public key, signature,
  address)`.
- Because the signed part holds the hash of every array, the address and the
  signature cover every byte of the manifest except the signature itself.
- The address cannot be stored inside the manifest, because a hash cannot
  hold itself. The header holds the manifest length, the date and the Merkle
  hash, and the address names the manifest from outside.

### Manifest arrays

This section defines the three arrays of version 1 and their order. A reader
skips an array kind it does not know, and its hash still counts, because the
hash is in the signed part.

The file list (kind 1) has one entry per file, sorted by path:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 0 | 32 | hash | SHA-256 of the whole file |
| 32 | 8 | u64 | file length |
| 40 | 8 | u64 | offset of the file in the linear packing |
| 48 | 4 | u32 | block size of this file, 512 by default |
| 52 | 4 | u32 | first block ID of the file |
| 56 | 4 | u32 | block count of the file |
| 60 | 32 | hash | Merkle hash over the block hashes of this file |
| 92 | 2 + n | path | file path |

A file entry carries its block size and its block hash list. The hash list
is the range of block IDs from the first block ID for the block count, and
the hashes themselves are in the block list at the end of the manifest, as
#14 asks. A file therefore has an offset and length in two places: within
the file (each block's offset in the block list) and within the linear
packing (the file's offset here, and each block's offset in the packing
map).

The block list (kind 2) has one entry of 48 bytes per block, in block ID
order:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 0 | 4 | u32 | block ID, counted from 1 |
| 4 | 32 | hash | SHA-256 of the block bytes |
| 36 | 4 | u32 | block length |
| 40 | 8 | u64 | offset of the block within its file |

The block packing map (kind 3) has one entry of 16 bytes per block, in block
ID order, and maps each block ID to its range in the linear packing:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 0 | 4 | u32 | block ID |
| 4 | 8 | u64 | offset of the block in the linear packing |
| 12 | 4 | u32 | block length |

Because the block entries have a fixed size, block ID n is at byte
`(n - 1) × 48` of the block list and `(n - 1) × 16` of the packing map, so a
client can read any range of them without reading the rest.

The block IDs, lengths and offsets are redundant with the file list. A
reader checks that they all agree and rejects the manifest otherwise.

A program of 64 KB has 129 blocks with its `cxapp.json`, which makes a
manifest of 8769 bytes (measured, see D30).

### Linear packing

This section defines the linear packing, which is the one byte sequence a
client fetches blocks from.

- All blocks of all files are laid out one after another in block ID order,
  with no padding. Since block IDs follow the file list, which is sorted by
  path, this is also every file in path order.
- A file with offset `o` and length `l` is the range `o` to `o + l` of the
  linear packing. The next file starts at `o + l`.
- Block `i` of a file (counted from 0) starts at `i × block size` within the
  file, and so at `o + i × block size` in the linear packing. Its length is
  the block size, except for the last block of the file, which holds the
  rest. An empty file has no blocks.
- Because a short last block is not padded, a block of the next file does
  not start at a multiple of 512 in the linear packing. The packing map
  gives every block's offset, so no client computes it.
- Any range of the linear packing can be asked for by offset and length. A
  client asks for whole blocks, so it can check each reply on arrival.

### Manifest chunking and fetching

This section says how the manifest itself is split and fetched, since it can
be larger than one packet.

- The manifest is fetched by offset and length with the get manifest
  operation, in pieces of at most one reply packet, 1154 bytes at the
  1200-byte packet size.
- The first piece holds the whole header. The client checks the magic, the
  version and the lengths, computes the address and compares it with the
  address it asked for, verifies the signature, and checks that the signer
  is trusted (decision D27). A manifest that fails any of these is dropped
  after one packet.
- The client then asks for the remaining pieces, several at a time. Each
  array is checked against the hash in the directory as soon as all its
  bytes are in. The file list is usually complete in the first piece.
- When the block list is complete, the client recomputes the Merkle hash of
  every file and of the whole manifest and compares them with the signed
  values.
- The get block list operation reads entries of the block list by block ID,
  so a device with little memory can read the hashes of one file at a time
  instead of holding the whole list.

### Merkle tree

This section defines the Merkle hash, which lets a client check a complete
set of block hashes against one signed value.

- A leaf is a block hash, the plain SHA-256 of the block bytes.
- An inner node is the SHA-256 of 65 bytes: the byte 0x01, the left child
  and the right child.
- The leaves are paired from the left. When a level has an odd number of
  nodes, the last node moves up to the next level unchanged. It is not paired
  with itself, which avoids the duplicate-leaf ambiguity of the Bitcoin tree
  (CVE-2012-2459).
- A tree with one leaf has that leaf as its root, and the root of an empty
  list is the SHA-256 of zero bytes.
- The header holds the root over all blocks of the manifest, and every file
  entry holds the root over its own blocks, so one file can be checked
  without the others.

### Text form

This section defines a text form of the manifest, which people can read and
tools can write, and which converts to the binary form byte for byte.

- The text form has one section per part, each opened by a `[name]` line,
  in the order `[header]`, `[arrays]`, `[files]`, `[hashes]`, `[linear]`.
- `[header]` has one `key = value` line per header field, plus `address`.
- Each other section has one entry per line. The fields are separated by
  commas and padded with spaces into aligned columns. The column order is
  fixed, and the first line of the section is a comment that names the
  columns.
- A line that starts with `;` is a comment. Numbers are decimal, and hashes,
  keys and signatures are lower case hex.
- Only a path is ever quoted, and only when it holds a comma, a space or a
  double quote, with `""` for a quote inside it.
- The address, the manifest length and the whole `[arrays]` section are
  derived from the other fields. A reader recomputes them from the binary
  form it rebuilds and rejects the text when they differ.
- The address and the signature are always computed over the binary form. A
  text form and its binary form therefore have the same address, and the
  text form is verified by converting it.

The columns:

- `[arrays]`: kind, count, offset, length, hash.
- `[files]`: path, file hash, size, offset in the linear packing, block
  size, first block ID, block count, Merkle hash.
- `[hashes]`: block hash, block ID, size, offset within the file.
- `[linear]`: block ID, offset in the linear packing, length.

This is the complete example from `tests/program-manifest-round-trip`,
whose test rebuilds it from `program/` and checks that it prints back the
same bytes. Its signer key is derived from the public seed `cx playground
example signer`, so it is for tests only. The program has two files, and
`main.cx` is 673 bytes, so it has two blocks, and its second block starts at
offset 637 of the linear packing, not at a multiple of 512.

```ini
; CX program manifest, text form. The address and signature are over the binary form.
[header]
magic           = CXPM
version         = 1
date            = 1795478400
manifest_length = 703
pack_length     = 798
merkle_hash     = ec58e0a6486748dd3e27041d757216637ea7dbefd55e07aa90dd54e62a0642a8
signer          = 039a645f9f2e3e101defd413e99b31a44c2e8f43620f3029bdd5454c652d6f0399
signature       = 76073fb71696b820525e17084b6b3ce45066bf15417597687bcf557979dd75e946e01dbddb37eae4bd97fad3dc9314b6fe99ea6d4e8cec2172e4e95ae347c4d901
address         = 33b4c5b0eecd6b75fe88fac07b24e7a31c024c027a74e0ea566390b8a908dd41

[arrays]
; kind, count, offset, length, hash
  1,    2,     306,    205,    6e402d956969a8a2318aff1356087f87c6d240c8077ea3130456c84e1a0558ba
  2,    3,     511,    144,    7df19b9d10ec8c9155008be599d248dfd8ffd3b878dcf9972266f2fc30aa6e35
  3,    3,     655,    48,     4e596b093b847948772757a8997d7038bc3ea14be0bbe75fc603fa889aeefc87

[files]
; path,       file_hash,                                                        size, offset, block_size, first_block, block_count, merkle_hash
  cxapp.json, 4755ead67638c13fd43f56cca93b8946074e3f86e0be0a1fe9da862722d2ef3f, 125,  0,      512,        1,           1,           4755ead67638c13fd43f56cca93b8946074e3f86e0be0a1fe9da862722d2ef3f
  main.cx,    aa24c98456232ee3a7d1f898e3eb1146caa23779dbd6cb2f2c55c1d61beda91f, 673,  125,    512,        2,           2,           adaea5e4a6e79ec088ec23db8c02c40594b78975ec68a5b0f5b636fbb142d48b

[hashes]
; block_hash,                                                       block_id, size, offset
  4755ead67638c13fd43f56cca93b8946074e3f86e0be0a1fe9da862722d2ef3f, 1,        125,  0
  d431f9aa898ad795ac4d3fdda0c34a8db75f9ec07d47b7e90158222c02db5a93, 2,        512,  0
  44f5c83df84986299209e4083f376d8c5694df314fd8ece10c48da6d2bd9f017, 3,        161,  512

[linear]
; block_id, offset, length
  1,        0,      125
  2,        125,    512
  3,        637,    161
```

### Verification of the files

This section lists the checks a client makes on the blocks and files, after
the manifest is verified.

- A block is accepted only when its SHA-256 matches its block list entry and
  its length matches.
- A file is accepted only when all its blocks are in, its length matches and
  the SHA-256 of the whole file matches its file list entry.
- The program is accepted only when every file is accepted.

### Relation to the JSON manifest

This section says how the new format sits next to the existing one.

- The JSON manifest stays for the apps embedded in the APK, and the binary
  format is used for programs fetched over the network. The loader of #17
  moves the embedded bundle to the binary format later. Decision D25 asks
  whether to replace the JSON manifest at once instead.
- The program metadata (name, kind, form, entry, arguments) stays in
  `cxapp.json`, which is an ordinary file of the binary manifest. A client
  rebuilds a `manifest.Manifest` from the downloaded files with
  `programblocks.AppManifest`, and runs it with `apprun.Run` as today.

## UDP operations (#15)

This part defines the packets that fetch a manifest list, a manifest and its
blocks, and how a client recovers from loss. Every packet is at most 1200
bytes, and a 1024-byte packet size is supported by a setting (decision
D26). The packets use the binary encoding conventions above.

### Packet header

This section defines the six bytes every packet starts with.

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 0 | 1 | u8 | protocol version, 1 |
| 1 | 1 | u8 | operation code; a reply adds 0x80; an error reply is 0xFF |
| 2 | 4 | u32 | request ID, chosen by the client and echoed in the reply |

### Operation packets

This section gives the layout of each request and reply after the six-byte
header. The largest counts are for a 1200-byte packet, with the 1024-byte
values in brackets.

List manifests (0x01) asks for the server's manifest list. The request has
no body. The reply body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 32 | hash | address of the manifest list |
| 38 | 4 | u32 | manifest length of the manifest list |

The manifest list is a manifest whose files are program manifests. Each
file's path is the program name, and its content is the program's binary
manifest. The client fetches the list like any program.

Get manifest (0x02) reads a range of a manifest. Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 32 | hash | manifest address |
| 38 | 4 | u32 | offset in the manifest |
| 42 | 2 | u16 | length wanted |

Reply body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 32 | hash | manifest address |
| 38 | 4 | u32 | offset |
| 42 | 4 | u32 | manifest length |
| 46 | n | bytes | manifest bytes, at most 1154 (978) |

Get block list (0x03) reads block list entries by block ID. This is the "get
hash list with length and offset" operation. Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 32 | hash | manifest address |
| 38 | 4 | u32 | first block ID |
| 42 | 2 | u16 | entry count wanted |

Reply body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 32 | hash | manifest address |
| 38 | 4 | u32 | first block ID |
| 42 | 2 | u16 | entry count |
| 44 | 48 × n | entries | block list entries, at most 24 (20) |

Get blocks (0x04) reads a range of the linear packing. Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 32 | hash | manifest address |
| 38 | 8 | u64 | offset in the linear packing |
| 46 | 2 | u16 | length wanted |

Reply body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 32 | hash | manifest address |
| 38 | 8 | u64 | offset |
| 46 | 2 | u16 | length |
| 48 | n | bytes | linear packing bytes, at most 1152 (976) |

A client asks for whole blocks, two 512-byte blocks per packet at 1200 bytes
and one at 1024.

Error (0xFF) is a reply only. Body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 1 | u8 | code: 1 unknown manifest, 2 range outside the manifest, 3 malformed request, 4 unsupported version |
| 7 | 1 | u8 | text length, at most 100 |
| 8 | n | bytes | text |

A server never sends more than the packet size. It shortens a reply that
would not fit, and the client asks again for what is missing.

### Loss, retries and order

This section says how the client handles lost, repeated and reordered
packets.

- The client keeps up to 16 requests in flight and matches each reply by
  its request ID. A reply to an unknown or finished request is dropped.
- A request with no reply after 250 ms is sent again, with the wait doubled
  each time up to 2 s, and the fetch fails after 8 tries of one request.
- Every request is idempotent, so a repeated reply does no harm.
- A block that fails its hash check is asked for again, and the fetch fails
  when the same range fails three times, because then the server holds bad
  data.

### Incremental verification

This section lists the order in which a client fetches and verifies a
program, so it never holds unverified data for long.

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
  document. It has fixed offsets and can be read by array without a parser.
- Option b: JSON with hex hashes. It is readable by eye, but a reader must
  parse all of it, hashes double in size, and the ESP32 needs a JSON parser.
- Recommendation: option a, because the manifest is read a packet at a time
  and on the ESP32. The text form of this document gives the readable view
  that JSON would, and D30 asks which of the two is canonical.

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

D30 ProgramManifestCanonicalFormDecision
- What exists now: the manifest has a binary form (`Manifest.Encode`,
  `programblocks.Decode`) and a text form (`Manifest.EncodeText`,
  `programblocks.DecodeText`) that convert to each other byte for byte. The
  address and signature are computed over the binary form.
- Question: are the address and signature computed over the binary form or
  over the exact bytes of the canonical text form?
- Option a: over the binary form, as built. Devices and packets use the
  binary form, and the text form is a view that tools convert.
- Option b: over the canonical text form. The text form is stored and sent,
  a tool or a small device writes it directly, and the binary form is dropped
  or kept only as a cache.
- Evidence, measured by `tests/program-manifest-form-sizes` on the host (an
  Intel i7-3770S), with gzip at its best level and lz4 block mode:

  | Manifest | Blocks | Binary | gzip | lz4 | Text | gzip | lz4 |
  |---|---|---|---|---|---|---|---|
  | apps/benchmarks | 21 | 2472 | 1780 | 1947 | 5157 | 2265 | 3817 |
  | 64 KB program | 129 | 8769 | 6087 | 6854 | 16313 | 7171 | 12787 |

  Decoding the 64 KB program's manifest takes 0.72 ms and 123 allocations
  (35 KB) in binary, and 1.03 ms and 1396 allocations (149 KB) from text.
  About 0.5 ms of both is the signature check. Decoding was not measured on
  an ESP32. The text form is about twice the binary size raw and 1.2 times
  compressed, because hex hashes compress back toward their raw size.
- Further facts: the binary form gives every block entry a fixed offset, so
  a device reads entry n at `(n - 1) × 48` without parsing the entries before
  it, and the header with the signature arrives in the first packet. A text
  form must be parsed line by line, its signed bytes depend on exact spaces
  and column widths, and a device needs decimal and hex parsing.
- Recommendation: option a, because the binary form is half the size raw,
  the ESP32 (about 300 KB of free RAM) reads it in place without the extra
  allocations, and its signed bytes cannot change with whitespace. Tools
  keep the text form for reading, writing and tests, and convert it with
  `DecodeText`.

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
    - ProgramManifestTextForm: the text form with a printer and a parser
      that convert to the binary form byte for byte. Depends on
      BinaryEncode. Test: program-manifest-round-trip with manifest.ini.
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
