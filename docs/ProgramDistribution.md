# Program distribution

This document describes how a CX program travels from the machine that
publishes it to a phone (and later an ESP32) that runs it. It covers the
signed manifest format (#14), the UDP operations that fetch a manifest and
its blocks (#15) and their forms for small packets (#31), the checks made
before a program runs (#16), the loader, lister and runner (#17), the DHT
prototype (#18) and the hash, memory, storage and serial test programs (#19
and skycoin-cx-forth #11). It ends with the open decisions and a task
breakdown down to tasks of about one pull request each.

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
- At a packet size of 352 bytes and up the first piece holds the whole
  header, and below that the header spans the first few pieces. Once the
  header is in, the client checks the magic, the version and the lengths,
  computes the address and compares it with the address it asked for,
  verifies the signature, and checks that the signer is trusted (decision
  D27). A manifest that fails any of these is dropped before the rest is
  fetched.
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

- The embedded bundle uses the binary format too, in the layout of the
  program store, as D25 option a planned once the store existed. The JSON
  form stays only as the output of `cxmanifest build` and `hash` and as the
  app manifest the app shows and runs.
- The program metadata (name, kind, form, entry, arguments) stays in
  `cxapp.json`, which is an ordinary file of the binary manifest. A client
  rebuilds a `manifest.Manifest` from the downloaded files with
  `programblocks.AppManifest`, and runs it with `apprun.Run` as today.

## UDP operations (#15)

This part defines the packets that fetch a manifest list, a manifest and its
blocks, and how a client recovers from loss. Every packet is at most the
packet size, 1200 bytes by default and a setting from 16 to 65507 bytes
(decision D26). The full forms (0x01 to 0x04) name a manifest by its whole
32-byte address. The short forms (0x05 to 0x08, #31) name a manifest or a
block by a hash prefix and read a part of one unit, so a reply fits a small
packet. The packets use the binary encoding conventions above.

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
| 6 | 1 | u8 | code: 1 unknown manifest or block, 2 range outside the unit, 3 malformed request, 4 unsupported version, 5 ambiguous prefix, 6 packet too small |
| 7 | 1 | u8 | text length, at most 100 |
| 8 | n | bytes | text |

A server never sends more than the packet size. It shortens a reply that
would not fit, and the client asks again for what is missing. When not even
the fixed part of a reply and one byte (or one block entry) fit, the server
answers with error 6, and it cuts the error text to fit.

### Short forms (#31)

This section defines the four requests that work below the floors of the
full forms. Each names a manifest or a block by a prefix of its hash, and
each reads an offset and length range inside one unit (the manifest, the
block list or one block), so the reply always fits.

A *prefix* is a u8 length L followed by the first L bytes of a manifest
address or a block hash. L is 4 to 32 bytes. A client sends 8 bytes by
default (D36) and fewer only when 8 do not fit the packet, never fewer than
4. Get block part also takes L = 0 for the manifest, which means any
manifest the server holds.

- A server refuses a prefix that matches two manifests, or two blocks whose
  full hashes differ, with error 5. Blocks with the same full hash hold the
  same bytes, so a prefix that matches several of them is not ambiguous.
- A client always checks the full value. It checks the address it computes
  from the header against the address it knows, or against the prefix when
  it knows only the prefix (the manifest list), and it checks every block
  against its full hash in the manifest. The signature and the trusted
  signer decide trust, as in the full forms, so a short prefix only names a
  unit and never makes a forged one acceptable.
- A client that gets error 5 can ask again with a longer prefix. With 8
  bytes, a server needs about 2^32 different blocks before two share a
  prefix by chance.

List manifests prefix (0x05) asks for the server's manifest list by prefix.
Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 1 | u8 | prefix length L wanted, 4 to 32 |

Reply body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 1 + L | prefix | prefix of the address of the manifest list |
| 7 + L | 4 | u32 | manifest length of the manifest list |

Get manifest by prefix (0x06) reads a range of a manifest. Get block list
bytes (0x07) has the same layout and reads a range of the block list array
of the manifest, so a device reads block entry n at `(n - 1) × 48` without
knowing where the array starts. Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 1 + L | prefix | manifest address prefix |
| 7 + L | 4 | u32 | offset in the manifest or the block list |
| 11 + L | 2 | u16 | length wanted |

Reply body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 1 + L | prefix | the request's prefix |
| 7 + L | 4 | u32 | offset |
| 11 + L | 4 | u32 | length of the manifest or of the block list |
| 15 + L | n | bytes | the bytes, at most packet size − 15 − L |

Get block part (0x08) reads a range of one block, named by its hash prefix
alone or with the manifest's prefix. Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 1 + M | prefix | manifest address prefix, M = 0 for any manifest |
| 7 + M | 1 + L | prefix | block hash prefix |
| 8 + M + L | 2 | u16 | offset in the block |
| 10 + M + L | 2 | u16 | length wanted |

Reply body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 1 + L | prefix | the request's block hash prefix |
| 7 + L | 2 | u16 | offset |
| 9 + L | 2 | u16 | block length |
| 11 + L | n | bytes | block bytes, at most packet size − 11 − L |

With 8-byte prefixes a get block part request is 28 bytes and its reply
carries 109 block bytes at 128 bytes, 45 at 64 and 13 at 32. A block longer
than 65535 bytes cannot be named this way, and the server answers error 3.

Which forms a client uses. The client's config flag names the forms, full
or short, and its packet size, and the client uses exactly those (D37).
Nothing switches forms or falls back to parts by itself.
`programblocks.Client.Forms` is `FormsFull` or `FormsShort`, the
`-forms full|short` flag of `cmd/programblocks` and `cmd/programnetwork`,
and the `forms=full|short` setting of a fetch app on the phone. The default
is full with a 1200-byte packet, the same packets as before #31. The short
forms need a server of #31; an older server answers them with error 3.

The full forms read whole units: a get blocks reply holds whole blocks and
a get peers reply whole peer entries. A request whose reply cannot hold one
whole unit at the configured size fails with error 6 (packet too small),
and the error names the flag to change, for example "a 512-byte block in a
get blocks reply at packet 512 in the full forms, raise the packet flag to
560 or more or set the forms flag to short". The same holds when a server
with a smaller packet size sends a get blocks reply that holds no whole
block. The short forms read parts: the client fetches each block in parts
and checks the block's hash when all its parts are in, so a short-form
client works with a server of any packet size
(`tests/program-transport-short-forms`).

### Packet size sweep (#31)

This section answers D26 with measurements.
`tests/program-packet-size-sweep` runs every operation over UDP on the host
at each packet size, with the server and the client at the same size and
the forms flag set for each row, and writes this table to `sweep-table.md`
in its folder. It also checks every operation's floor, the smallest size
its packet layout allows, against the runs: every size at or above the
floor works and every size below it fails with error 6, packet too small.

The reference program is `apps/benchmarks`: 8 files, a manifest of 2472
bytes, 21 blocks and 8599 bytes of linear packing. A cell is the packets
(requests and replies) and bytes (headers included) for one fetch: the
manifest list address for list manifests, the program manifest for get
manifest, all 21 block entries for get block list, the whole linear packing
for get blocks, and the whole program from list to verified files for fetch
program. The DHT rows are one flood of the program from a node to its
subscriber (subscribe, have, and the subscriber's download), and one get
peers of five public peers, the last of them a 99-byte entry with a 64-byte
address and a 32-byte group. Retransmissions are not counted, and there
were none.

| Operation | 1400 | 1200 | 1024 | 512 | 256 | 128 | 64 | 32 | 16 | Floor | Below the floor |
|---|---|---|---|---|---|---|---|---|---|---|---|
| list manifests 0x01 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | 2/48 | fails | fails | 42 | reply: header 6, 32-byte address, length 4 |
| list manifests prefix 0x05 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/26 | 2/23 | 15 | reply: header 6, prefix 1+4, length 4 |
| get manifest 0x02 | 4/2652 | 6/2742 | 6/2742 | 12/3012 | 24/3552 | 62/5262 | 276/14892 | fails | fails | 47 | request 44 with the 32-byte address, reply 46 before the first byte |
| get manifest by prefix 0x06 | 4/2560 | 6/2604 | 6/2604 | 12/2736 | 22/2956 | 48/3528 | 122/5156 | 550/14572 | fails | 20 | reply: header 6, prefix 1+4, offset 4, length 4, one byte |
| get block list 0x03 | 2/1096 | 2/1096 | 4/1184 | 6/1272 | 12/1536 | 42/2856 | fails | fails | fails | 92 | reply 44 with the 32-byte address, plus one 48-byte block entry |
| get block list bytes 0x07 | 2/1052 | 2/1052 | 4/1096 | 6/1140 | 10/1228 | 20/1448 | 50/2108 | 224/5936 | fails | 20 | as get manifest by prefix |
| get blocks 0x04 | 16/9367 | 18/9463 | 28/9943 | fails | fails | fails | fails | fails | fails | 560 | reply 48 with the 32-byte address, plus one whole 512-byte block |
| get block part 0x08 | 42/9586 | 42/9586 | 42/9586 | 68/10197 | 102/10996 | 174/12688 | 408/18187 | 1350/40324 | 17198/275168 | 16 | reply: header 6, block prefix 1+4, offset 2, length 2, one byte |
| fetch program, forms=full | 26/12995 | 28/13091 | 40/13667 | fails | fails | fails | fails | fails | fails | 560 | get blocks |
| fetch program, forms=short | 56/13093 | 56/13093 | 56/13093 | 92/13936 | 140/15058 | 238/17352 | 562/24933 | 1902/56231 | fails | 20 | get manifest by prefix |
| subscribe 0x11 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 2/20 | 10 | request and reply: header 6, lease 4 |
| have 0x10 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | 2/88 | fails | fails | 50 | request: header 6, 32-byte address, lengths 4 and 8 |
| flood one program, forms=full | 14/11655 | 16/11749 | 21/11989 | fails | fails | fails | fails | fails | fails | 560 | get blocks |
| flood one program, forms=short | 27/11624 | 28/11647 | 28/11647 | 44/11963 | 66/12401 | 115/13384 | 269/16458 | fails | fails | 50 | the have |
| get peers 0x12, forms=full | 2/228 | 2/228 | 2/228 | 2/228 | 2/228 | 4/247 | fails | fails | fails | 110 | reply 11 plus the longest peer entry, 99 |
| get peer part 0x13, forms=short | 10/314 | 10/314 | 10/314 | 10/314 | 10/314 | 10/314 | 12/335 | 26/482 | 108/1343 | 13 | reply: header 6, total 2, index 2, length 1, offset 1, one byte |

Findings:

- No operation fails because of the 65-byte signature or the 33-byte signer
  key. The client puts the 306-byte header together from as many get
  manifest replies as it takes (at 352 bytes and up it is one reply) and
  checks the address, the signature and the signer when its last byte
  arrives.
- The 32-byte address in every full-form packet sets their floors: 42 for
  list manifests, 47 for get manifest and 50 for have. Get block list also
  needs one 48-byte block entry in a reply, 92 bytes.
- The full forms read whole units (D37). Get blocks needs a whole 512-byte
  block in a reply, 560 bytes, so a program fetch and a flood in the full
  forms work at 560 and up and fail with error 6 below.
- Get peers needs room for its longest entry, 110 bytes for a 99-byte
  entry. Get peer part reads an entry by range, down to 13 bytes, when the
  forms flag is short.
- In the short forms the floor of every fetch operation is set by the
  6-byte header, the 5-byte shortest prefix and the offset and length
  fields: 15 for list manifests prefix, 20 for get manifest by prefix and
  get block list bytes, 16 for get block part. A whole program fetch works
  down to 20 bytes, where 16 fails at get manifest by prefix.
- The flood needs the have, so its floor in the short forms is 50 bytes
  (D35).
- In the short forms every operation works at 64 bytes and up. That is the
  practical floor. At 32 bytes a program can still be fetched from a
  server, in 1902 packets instead of 56 at 1200, but a node cannot flood.
  At 16 no program can be fetched.
- Cost: in the full forms fetching the program takes 28 packets and 13091
  bytes at 1200. In the short forms it takes 56 packets and 13093 bytes at
  1200, 238 packets and 17352 bytes at 128, 562 and 24933 at 64, and 1902
  and 56231 at 32 (4.3 times the bytes).
- At 1200 the short forms send about the same bytes in twice the packets,
  because a get block part never spans two blocks. So the forms flag
  defaults to full.

### Loss, retries and order

This section says how the client handles lost, repeated and reordered
packets.

- The client keeps up to 16 requests in flight and matches each reply by
  its request ID. A reply to an unknown or finished request is dropped.
- A request with no reply after 250 ms is sent again, with the wait doubled
  each time up to 2 s, and the fetch fails after 8 tries of one request.
  The tries are a setting: a fetch app takes `tries=N` (and the packet
  size as `packet=BYTES` and the forms as `forms=full|short`) after its
  three args, and
  `tests/program-transport-fetch-in-emulator-with-loss` makes 20 (#31).
  Under its 40% loss each way a request and its reply both arrive
  36% of the time, so 8 tries lost a request once in 36 and the fetch of
  about 4 requests failed about once in 9 runs. 20 tries lose a request
  once in 7500, and the loss stays 40%, so the test still shows recovery
  under heavy loss.
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
3. For the chosen program, take its manifest from the list, which the
   list's file hash has already verified, and check its signature and
   signer. A client that only knows a program's address uses get manifest
   instead, as in step 1.
4. Get blocks two at a time from the linear packing, checking each block on
   arrival and each file when it is whole.

## Validation before running (#16)

This section describes the check made on a downloaded program before it
runs, and how the result is shown. Package `programcheck` holds the checks
(#25), and the app shows the report before a fetched program runs.

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
on the phone, and the path to the ESP32. `programstore` holds the storage
layer (#26) and an app of kind `programs` opens the screen.

### Screen

The screen stays monospaced at one text size, as today. Each row shows the
program number, the name, whether it is remote, partial or cached, with
`+ram` when the loader holds it, and its length. Tapping a row selects it,
and `[ download ]`, `[ validate ]` and `[ run ]` act on it, with
`[ unload ]` to drop it from RAM and `[ clear ]` to remove its packing too.
Validate loads the program into RAM and shows its report under the
buttons. Run starts only a program whose report allows it, and leaving the
program returns to the list. `[ refresh ]` fetches the server's list again.

An app of kind `programs` opens the screen. Its args are the server address
and the signer keys to trust, which are added to `programs/trusted.txt`.

### Storage layout

The SD card (or the app's files directory on the phone) holds, as package
`programstore` writes it:

- `programs/list.bin`, the last verified manifest list;
- `programs/m/<address>.bin`, one verified manifest per program;
- `programs/p/<address>.pack`, the program's linear packing, written block
  by block as blocks are verified, so a stopped download resumes;
- `programs/index.txt`, one line per program with its number, address,
  state (`remote`, `partial` or `cached`), length and name, in that order so
  the name may hold spaces, which the lister reads without opening any
  manifest, for example
  `1 <address in 64 hex digits> cached 9216 benchmarks`;
- `programs/trusted.txt`, the trusted signer keys of D27, one key in hex
  per line with an optional comment after it.

A packing file has no record of which blocks it holds. Opening it reads
every block back and keeps the ones whose hash matches, and a block never
written reads as zeros and fails, so the store needs no second file that
could disagree with the packing. Blocks are synced every 32 writes and at
the end, so a crash loses at most those, and they are fetched again.

The lister reads `index.txt` with one sequential read, and every row's
strings share the one copy of the file, so the allocations of a read do not
grow with the number of rows. The loader (`programstore.Memory`) reads the
manifest and the packing into RAM, verifying every block and file, and
keeps at most a set number of programs. The unloader drops them. The runner
runs what the loader holds.

### Memory budget

A phone has room for every program. An ESP32 has about 300 KB of free RAM,
so there the loader keeps at most one program in RAM, reads blocks from the
SD card as needed, and the manifest's block arrays are read in pieces rather
than held whole.

### ESP32 path

This section is the design for the loader on an ESP32. Nothing of it is
built yet, and nothing here was run on a board.

The goal is the phone's flow on a board with an SD card, a serial console
or a small display, and Wi-Fi: list the programs of a server, download one
to the SD card, check it, load it and run it.

Runner. The runner is cx-forth (skycoin-cx-forth), which already runs on
the ESP32 and translates a subset of the CX IR to Forth. A program for the
board is published in the Forth form, or as serialized IR that the board
translates once after the download and keeps as Forth. A `cx build` core
(skycoin-cx #496 links only the system libraries a program uses) is the
second runner, for programs that need more than the subset, once a core fits
in flash next to the loader.

Fetcher. The fetcher is the `programblocks` client built with TinyGo. It
needs a UDP socket on the board's Wi-Fi stack, which TinyGo does not give on
the ESP32 today, so the first board version fetches over the serial line
through a host that runs `programblocks serve` and relays packets, with the
same packets and checks. The relay is a few lines on the host and changes
nothing in the format.

Store. The SD card holds the same layout as the phone, `programs/list.bin`,
`m/`, `p/`, `index.txt` and `trusted.txt`, on FAT, which method A of the
storage tests (#19, skycoin-cx-forth #11) measures. A packing is written
one 512-byte block at a time, which is one SD sector, and synced every 32
blocks, as on the phone. If FAT proves too slow or too large, method B
keeps one preallocated packing area of raw sectors and the index in its
first sector.

Memory. About 300 KB of RAM is free. The loader keeps one program in RAM
(`programstore.Memory` with a limit of 1). It does not decode a whole
manifest: the header and the array directory are read first, and a block
entry is read at `(n - 1) * 48` inside the block array when it is needed,
which the fixed offsets of the binary form allow (D30). Resuming reads one
block at a time to check its hash, so it needs one 512-byte buffer.
cx-forth's stacks and memory have fixed sizes, set when the program loads.

Check. The checks of `programcheck` read the compiled IR, which the board
does not hold. The board trusts the signer key list in `trusted.txt` and
runs only programs whose manifest names the Forth form, and cx-forth has no
file, network or process words unless the board adds them. The check
report of the serialized IR can be made on the host and published as a file
of the program, so the board shows it without computing it.

Screen. The list is the same monospaced rows over the serial console or on
the display: number, name, state, length. Keys select a row, then download,
check, load and run, as the taps do on the phone.

Order of work: the serial relay and the TinyGo build of the client, the FAT
store with the resume test in QEMU, the one-program loader with the Forth
runner, then the board, where the timings of #19 are measured.

### Suspend and resume (later)

Saving the VM state (stacks, memory, registers, open handles) to the SD
card allows pause, suspend, resume and switching programs. cx-forth has
fixed-size stacks and memory, so its state is a fixed set of byte ranges
plus the program address, which makes it the first target. Each save and
load is timed and reported.

## DHT prototype (#18)

This section describes the prototype network that spreads programs between
nodes. The `programnetwork` package holds the node and the peer list, and
`cmd/programnetwork` runs a node and manages its peer list file. A node
keeps the programs it holds in a `programstore` store and serves them with
the operations of #15, so any node that sent a have is a server.

### Packets

The DHT reuses the six-byte packet header of #15 and adds four operations.
Every packet is at most the packet size, 1200 bytes by default, a setting.

Have (0x10) announces that the sender holds a program. Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 32 | hash | manifest address |
| 38 | 4 | u32 | manifest length |
| 42 | 8 | u64 | linear packing length |

The request is 50 bytes. The reply (0x90) acknowledges it with the 32-byte
manifest address, 38 bytes in all. The sender sends the have again until the
reply arrives, with the waits of #15, so a lost have does not end a flood.

Subscribe (0x11) asks the receiver to send have packets to the sender's
address. Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 4 | u32 | lease wanted in seconds, 0 ends the subscription |

The reply (0x91) has the same body with the lease granted, at most 600
seconds by default. The subscriber renews at half the lease. Both packets
are 10 bytes.

Get peers (0x12) asks for the peers the receiver shares with the sender.
Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 2 | u16 | index of the first peer wanted |

Reply (0x92) body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 2 | u16 | number of peers shared with the asker |
| 8 | 2 | u16 | index of the first entry |
| 10 | 1 | u8 | entry count |
| 11 | n | entries | peer entries |

A peer entry is a u8 address scope (1 LAN, 2 internal, 3 global), a u8
address length, the address as `host:port` text of at most 64 bytes, a u8
group name length and the group name of at most 32 bytes. An entry is at
most 99 bytes, so a reply always holds at least 12 entries at 1200 bytes and
10 at 1024. The reply holds as many entries as fit, and the asker asks again
from the next index until it has them all.

Get peer part (0x13, #31) reads a range of one encoded peer entry, so an
entry longer than a get peers reply can hold still arrives. Request body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 2 | u16 | index of the peer |
| 8 | 1 | u8 | offset in the encoded entry |

Reply (0x93) body:

| Offset | Size | Type | Meaning |
|---|---|---|---|
| 6 | 2 | u16 | number of peers shared with the asker |
| 8 | 2 | u16 | index of the peer |
| 10 | 1 | u8 | length of the encoded entry, 0 for an index past the number |
| 11 | 1 | u8 | offset |
| 12 | n | bytes | entry bytes, at most packet size − 12 |

`programnetwork.Node.Forms` is the node's forms flag, as in the client
(D37). With `FormsFull` a node reads whole entries with get peers, and an
entry that does not fit a reply is error 6 naming the flag. With
`FormsShort` it reads every entry with get peer part, down to 13 bytes. A
node's downloads use the same flag, so in the short forms a flood works at
every size the have fits, 50 bytes and up, and in the full forms at 560
and up.

A malformed request gets the error reply (0xFF) of #15. The download uses
get manifest and get blocks of #15 against the node that sent the have.

### Flood replication

When a node receives a have for a manifest address it has not seen, it
records the address as seen, acknowledges the have and downloads the
program from the node that sent it. It fetches the manifest, checks the
signer against its trusted keys and the lengths against the have, keeps the
manifest and the blocks in its store as each is verified, and checks every
file. Then it holds the program and sends its own have to every subscriber.

A node announces a program at most once, and a have for a seen address only
counts as a duplicate, so a flood over any graph ends after one have per
subscription and program. A download that fails is tried again from the
other nodes that announced the program, three times in all, and then the
address is no longer seen, so a later have starts over. A program from an
untrusted signer, or one whose lengths differ from its have, is dropped and
never forwarded.

A new subscriber gets a have for every program the node already holds
(D34), so a node that joins late, such as a phone, catches up.

Measured by `tests/program-network-flood-in-process` on the host, with each
node subscribed to its two ring neighbours and the node three places on,
and three programs published at node 0:

| Nodes | All held after | Downloads | Haves received | Duplicates |
|---|---|---|---|---|
| 3 | 88 ms | 6 | 18 | 12 |
| 8 | 389 ms | 21 | 72 | 51 |
| 16 | 673 ms | 45 | 144 | 99 |

Every node downloads every program once, and the haves received are exactly
one per subscription and program. `tests/program-network-flood-loss-tolerance`
runs eight nodes whose sockets drop a share of every read and every write
and send a tenth of the rest twice. With 20% dropped, which loses 59% of the
have and subscribe round trips, every node held both programs after 12.8 s.
With 30%, which loses 76% of them, it took 28.0 s. Each node still
downloaded each program once.

### Peer list file

A node's peer list is a text file, `peers.txt` by default. Each line is a
group, a peer, a comment starting with `#`, or empty:

```
# group NAME SCOPE OWNER OPERATOR SECURITY-GROUP
group china-internal internal lab isp-b china
group china1 internal lab isp-a china
group global global - - global
group lan1 lan moe moe home
# peer ADDRESS GROUP CLASS [COMMENT]
peer 10.1.0.1:7010 china1 own-infrastructure
peer 192.168.1.5:7010 lan1 lan-group laptop in the office
peer 203.0.113.5:7010 global public
```

A group has a name of at most 32 characters, an address scope (`lan`,
`internal` or `global`), an owner, an operator and a security group, each a
single word with `-` for none. A peer has its node's address, its group and
one of the five sharing classes:

1. `private`: never shared;
2. `own-infrastructure`: shared only with peers that are themselves
   `own-infrastructure` records, the owner's own nodes;
3. `lan-group`: shared only with peers of the same group;
4. `user-shareable`: shared with every peer in the list (D31);
5. `public`: shared with anyone, also a node not in the list.

The rest of a peer line is a comment. A tool writes the groups and then the
peers, each sorted, so the file round trips byte for byte.

### Sharing rules

A get peers reply holds a peer only when both its class and its group's
scope allow the asker. The asker's record is the one whose address is the
packet's source address, or else the only record on the same host (D32). An
asker with no record is a stranger in the `global` group.

- A LAN address goes only to an asker of the same group.
- An internal address goes only to an asker whose group has the same
  security group, such as `china1` and `china-internal` (D33).
- A global address goes to anyone.

The asker's own record is never returned to it.
`tests/program-network-get-peers-share-classes` and
`tests/program-network-get-peers-groups-scopes` check each class and scope
over UDP.

### Command

`programnetwork peers` manages the file, one change per run:

```
programnetwork peers -file peers.txt group-add lan1 lan moe moe home
programnetwork peers -file peers.txt add 192.168.1.5:7010 lan1 lan-group laptop
programnetwork peers -file peers.txt set-class 192.168.1.5:7010 user-shareable
programnetwork peers -file peers.txt set-group 192.168.1.5:7010 global
programnetwork peers -file peers.txt remove 192.168.1.5:7010
programnetwork peers -file peers.txt group-remove lan1
programnetwork peers -file peers.txt list
programnetwork peers -file peers.txt shared-with 203.0.113.9:7010
```

`programnetwork node` runs a node: `-store` names its store, `-peers` its
peer list, `-signer` the trusted keys, `-subscribe` the nodes it subscribes
to, and `-publish DIR` with `-seed` or `-secret` publishes every program
directory in DIR and floods it. `programnetwork getpeers HOST:PORT` prints
what a node shares with the caller.

### On the phone

An app of kind `flood` runs a node on the phone with an empty store in the
app's files dir. Its args are the nodes to subscribe to, the trusted signer
key and the name in the wanted program's `cxapp.json`. It waits up to 30 s
for that program to arrive by flood, then checks and runs it like a `fetch`
app.

`tests/program-network-flood-in-emulator` starts three nodes in the rig
container with the `flood-nodes` setting of the emulator driver: 7010
publishes `fib-and-primes`, 7011 subscribes to 7010 and 7012 to 7011, and
the app starts once 7012 holds the program. The app subscribes to 10.0.2.2
ports 7012 and 7011, gets a have from each (D34), downloads the program
once, verifies every block and runs it. In the first run the program was
held 27 ms after the app subscribed, with 2 haves received, 1 duplicate and
1 download, and the whole test took 7.1 s including install and start.

### Node types (#34)

A node has one of three types, which decide what it downloads, keeps,
announces and serves. Only a node that holds a program announces it and
serves it, so a have always names a node that can answer the download.

| | Thin | Medium | Super |
|---|---|---|---|
| on a have | records it, downloads nothing | downloads when pinned or within the budget | downloads when it fits the capacity |
| keeps for others | nothing | pinned programs and others within the budget | everything until full |
| announces and serves | nothing | what it keeps | what it keeps |
| subscribers | grants no lease | yes | yes |
| survives a restart | nothing | programs, subscriptions, subscribers | programs, subscriptions, subscribers |

A thin node subscribes like any node, so it hears every have and knows
which programs exist and which nodes hold them (D39). `Node.Known` lists
them, and `Node.Fetch` downloads one on demand from the nodes that
announced it or from nodes given by address. The thin node keeps what it
fetched in its own store, for itself: it never adds the program to what it
serves, never announces it, and answers a get manifest for it with error
1. A second fetch of a cached program reads the store.

A medium node keeps every program its pin list names, and other programs
while the bytes it holds stay within its budget (D38). The bytes are the
manifest and the linear packing of each program held, which is what the
store holds on disk. A download reserves its bytes when it starts, so haves
that arrive together cannot overcommit the budget. Pinned programs count
against the budget and are kept past it. With no budget a medium node keeps
only pinned programs. The pin list is a text file, one manifest address or
address prefix of 8 to 64 hex digits per line, `#` starting a comment line.
The node reads it again whenever it changes, so a pin added while the node
runs applies to the next have.

```
# pins.txt: one address or address prefix per line
4bf42c1844ab4576
```

A super node keeps every program it hears of while it fits the capacity,
the `-budget` of a super node, and with no capacity until a write fails
because the disk is full. There is no eviction. A program that does not
fit is refused with a "storage full" log line and counted in
`Stats.HavesRefusedFull`, and the node keeps serving what it holds (D40).
A smaller program that still fits is taken. A write that fails with no
space left drops the partial program from the store. Removing a program is
manual, with `programnetwork store remove`; a running node keeps serving
it from memory until it restarts.

A medium or super node keeps two files next to its store's `index.txt`:

```
# subscriptions.txt: the nodes this node subscribes to, one per line
10.1.0.1:7010
# subscribers.txt: this node's subscribers and their lease ends, RFC 3339
192.168.1.5:7010 2026-11-24T03:29:46Z
```

On start the node holds every cached program of its store, takes back the
subscribers whose lease has not ended, so a program held after a restart
reaches them before they renew, and subscribes again to every node of
`subscriptions.txt`. Such a resumed subscription first ends the earlier one
with a lease of 0, so the other node, which still counts this node as a
subscriber, sends the catch-up haves of D34 for what was published while
it was down. A thin node writes neither file.

`programnetwork node -type thin|medium|super -budget BYTES -pins FILE`
sets the type, super by default as before #34 (D41). `-config FILE` reads
the same three settings from `type`, `budget` and `pins` lines, and a flag
given on the command line overrides its line. `-fetch HASH,...` fetches
programs on demand from the `-subscribe` nodes; a thin node exits after
them. The pin list and the store have their own commands:

```
programnetwork pins -file pins.txt add 4bf42c1844ab4576
programnetwork pins -file pins.txt remove 4bf42c1844ab4576
programnetwork pins -file pins.txt list
programnetwork store -store DIR list
programnetwork store -store DIR remove 4bf42c18
```

`store remove` takes an address prefix of at least 8 hex digits, refuses
one that matches two programs, and deletes the manifest, the packing and
the index row.

Tests, each over UDP on the host:

- `tests/program-network-node-types-mixed`: a publisher, a super node, a
  medium node with one pin, a medium node with a budget, a thin node, and a
  super node subscribed only to the pinned medium node. After three
  programs of 11071, 1225 and 2293 bytes, the super node holds all three,
  the pinned medium node only its pin, the budget medium node one program
  within its 2293 bytes, the node behind the pinned medium node only the
  pinned program, and the thin node none while it knows all three. The
  thin node then fetches one from the super node in about 30 ms, still
  holds nothing, and serves nothing.
- `tests/program-network-super-full-no-eviction`: a super node with a
  capacity of two programs keeps both, refuses the third, still serves the
  first, and after a manual remove and a restart takes the removed one
  again from the catch-up haves and still refuses the third.
- `tests/program-network-node-restart-persistence`: a super node and a
  medium node restart on their ports. Both hold what they held, the super
  node knows its subscriber before the medium node is back, neither calls
  Subscribe, and a program published after the restart, pinned while the
  medium node ran, reaches both.
- `tests/program-network-node-budget-accounting`: six programs published
  at once to a medium node with a budget and one pin larger than the
  budget. The unpinned programs held stay within the budget, the pin is
  held past it, the bytes counted equal the bytes on disk, and a restart
  counts the same.

On the phone, an app of kind `thin` runs a thin node with an empty store
in the app's files dir. Its args are those of a `flood` app. It subscribes
to the nodes, fetches each program it hears of on demand until one has the
wanted name, and checks and runs it like a `fetch` app. The emulator
driver's `flood-type` setting gives the rig's flood nodes their `-type`.
`tests/program-network-thin-fetch-in-emulator` runs one super node on port
7020 in the rig container that publishes `fib-and-primes`. The app
subscribes to it at 10.0.2.2, hears one have and ignores it, fetches the
program on demand with one download, holds nothing for others, has no
subscriber, verifies every block and runs it. In the first run the program
was fetched 89 ms after the app subscribed, and the whole test took 5.4 s
including install and start.

## Test programs (#19 and skycoin-cx-forth #11)

This section describes the hash, memory, storage and serial test programs
and how their results are reported. The phone side is built, see "Test
programs" in the README. The ESP32 side is skycoin-cx-forth #11.

### Report

Every test prints one line, read and written by package `testreport`:

```
name verdict blocks_per_s kb_per_s latency_us [serial_read_kb_s serial_write_kb_s] [note]
```

The verdict is `true`, `false` or `skipped`, a measurement is a number or
`-`, and the serial speeds are present exactly for the `Serial` tests. Test
names are lexicographic and hierarchical, and a storage name carries its
method and block size:

- `HashSha256Known`, `HashMerkleKnown`;
- `MemoryPatternAddress`, `MemoryPatternMovingInversions`,
  `MemoryPatternRandomSeeded`, `MemoryPatternWalkingOnes`,
  `MemoryPatternWalkingZeros`, `MemoryCorruptionDetectedBitFlip`;
- `StorageFileSystemRead512` to `StorageFileSystemWrite4096` (method A),
  `StorageDirectBlockRead512` to `StorageDirectBlockWrite4096` (method B),
  `StorageCorruptionDetectedByteFlip`;
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
  bundle tests unchanged while the network format settles. Done: the
  bundle moved to the binary manifest after the store of #17.

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
- Lead 2026-11-24: test 1400, 1024, 512, 256 and 128, then 64, 32 and 16,
  add partial-hash and sub-block requests, keep 1200 until the results.
- Results (#31): "Packet size sweep" above. 1200 stays the default and the
  size is a setting from 16 to 65507. Every operation works at 64 bytes and
  up, a program fetch from a server works down to 20, and the flood down to
  50. D35 to D37 ask the choices this left open.

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

D31 PeerUserShareableClassDecision
- What exists now: the issue lists five sharing classes. Private, own
  infrastructure, LAN group and public have a clear audience. User-selected
  shareable does not say who it is shared with.
- Question: who receives a `user-shareable` peer in a get peers reply?
- Option a: every asker that has a record in the peer list, whatever its
  class or group. A stranger does not get it, which is what sets it apart
  from `public`.
- Option b: only askers the user names for that peer, a share list kept on
  the peer line.
- Option c: anyone, the same as `public`, with the class kept only as a
  label of who chose to share it.
- Recommendation: option a, because it needs no extra field, it differs
  from `public` in a way a node can check, and it matches "the user chose to
  share this with the peers the user knows". Built: `classAllows` in
  `programnetwork/peers.go`.

D32 PeerAskerGroupSourceDecision
- What exists now: the sharing rules need the asker's group, and a get peers
  packet carries no identity.
- Question: how does a node learn the group of the node asking it?
- Option a: from its own peer list, by the packet's source address, or the
  only record on the same host. An asker with no record is a stranger in
  `global`.
- Option b: the asker names its group in the request, and the node believes
  it.
- Option c: the asker signs the request with its node key, and the node
  looks the key up in its peer list, which then holds keys.
- Recommendation: option a for the prototype, because a claimed group lets
  anyone read a LAN group's peers, and the source address is what a UDP
  node already has. Option c is the step after, when peer records gain
  keys. Built: `PeerList.Asker` in `programnetwork/peers.go`.

D33 PeerInternalScopeReachDecision
- What exists now: the issue names LAN, internal and other scopes, and
  groups such as `china1` and `china-internal` with a security group.
- Question: which askers may receive an address of internal scope?
- Option a: askers whose group has the same security group, so `china1`
  and `china-internal` share addresses when both have security group
  `china`.
- Option b: askers of the same group only, the same as a LAN address.
- Option c: any asker that is not a stranger.
- Recommendation: option a, because it gives the security group a job and
  lets one private network hold several groups with different operators.
  LAN addresses stay within their group. Built: `scopeAllows` in
  `programnetwork/peers.go`.

D34 FloodSubscribeCatchUpDecision
- What exists now: a have is sent when a node starts to hold a program, so
  a node that subscribes later hears nothing about programs already spread.
- Question: does a node send haves for what it holds when a peer
  subscribes?
- Option a: yes, one have per held program to each new subscriber, once,
  not on a renewal.
- Option b: no, a late node asks for a list of held programs with a new
  operation.
- Option c: no, a late node waits for the next new program.
- Recommendation: option a, because it reuses the have and its download
  path, a phone that starts the app after the flood still gets the
  programs, and it costs one 50-byte packet per program per new subscriber.
  Built: `Node.subscribe` in `programnetwork/node.go`.

D35 ProgramPacketSizeFloorDecision
- What exists now: after #31 every operation works at 64 bytes and up. In
  the short forms a program fetch from a server works down to 20 bytes. The
  have (0x10) is 50 bytes because it carries the 32-byte manifest address,
  so a flood needs 50 bytes. At 16 bytes no program can be fetched, because
  the header, the shortest prefix and the offset and length fields of get
  manifest by prefix are 19 bytes before the first manifest byte.
- Question: which packet size is the supported floor?
- Option a: 64 bytes for every operation, the flood included, with the have
  kept as it is. 32 bytes works for fetching from a server but is not
  promised for the flood.
- Option b: 32 bytes for every operation, by adding a short have that names
  the program by an address prefix. The receiver then fetches the manifest
  by prefix and learns the full address only from its header, so the seen
  set and the announcers are keyed by prefix until then.
- Option c: 128 bytes, the networks with a 128-byte maximum that #31
  names, with the sizes below 128 left as untested extras.
- Evidence: the sweep table. The smallest links named so far are 802.15.4
  (127-byte frames) and LoRa at its slowest data rates (51-byte payloads),
  and both carry the 50-byte have. A fetch at 32 bytes takes 1902 packets
  and 4.3 times the bytes of a fetch at 1200.
- Recommendation: option a, because every link named so far carries 64
  bytes, the have fits LoRa's 51 bytes, and option b changes how the flood
  keys its state for links nobody has asked for. Built: the floors in
  `tests/program-packet-size-sweep`; nothing else needed.

D36 ProgramHashPrefixLengthDecision
- What exists now: #31 asks for blocks by a partial hash and does not say
  how long the prefix is.
- Question: how long is a manifest address or block hash prefix, and what
  happens when one matches two units?
- Option a: a u8 length from 4 to 32 bytes on the wire, 8 bytes sent by
  default and fewer only when 8 do not fit, the server refuses a prefix
  that matches two different units with error 5, and the client checks
  every full hash.
- Option b: a fixed 4-byte prefix, which saves 4 bytes per packet and makes
  a collision likely past about 65000 blocks on one server.
- Option c: a fixed 16-byte prefix, which never collides in practice and
  costs 8 more bytes per packet than option a.
- Recommendation: option a, because 8 bytes need about 2^32 blocks on one
  server before two collide by chance, the length byte lets a client go
  shorter on a tiny packet or longer after error 5, and trust never rests
  on the prefix since the client checks the full hash and the signature.
  Built: `MinPrefixLength`, `DefaultPrefixLength` and `Client.PrefixLength`
  in `programblocks`, tested by `tests/program-transport-short-forms` with
  two blocks that share 4 bytes.

D37 ProgramShortFormSelectionDecision
- Decided 2026-11-24 (#43): "Client chooses, client config flag. Based
  upon client choose. No automatic." The client uses exactly the forms and
  packet size its config names, and a reply that cannot fit fails with
  error 6 naming the flag. Built: `Client.Forms` and `Node.Forms`
  (`FormsFull` by default, `FormsShort`), the `-forms` flag and the
  `forms=` fetch setting, see "Short forms". `FormsAuto` and the fallback
  to block parts and peer parts are removed.

D38 MediumNodePinBudgetDecision
- What exists now: #34 says a medium node stores up to a configured amount
  and/or pins certain hashes, and does not say how the two combine.
- Question: do pinned programs count against a medium node's budget, and
  is a pinned program kept when it does not fit?
- Option a: pinned programs are always kept and count against the budget;
  an unpinned program is kept only while all bytes held stay within the
  budget. With no budget the node keeps only pinned programs.
- Option b: the budget covers unpinned programs only, and pinned programs
  come on top of it.
- Option c: the budget is a hard cap for everything, so a pin that does not
  fit is refused.
- Recommendation: option a, because the budget then bounds the disk the
  node gives to programs it was not asked for, a pin is the operator's
  explicit wish and is never refused, and one number in `Stats.StoredBytes`
  answers both. Built: `Node.onHave` in `programnetwork/node.go`, tested by
  `tests/program-network-node-budget-accounting`.

D39 ThinNodeSubscribeAndCacheDecision
- What exists now: #34 says a thin node gets data only and fetches what it
  needs.
- Question: does a thin node subscribe, and does it keep what it fetched?
- Option a: it subscribes, so it hears every have and knows what exists and
  where, downloads nothing on a have, and keeps what it fetches in its own
  store for itself without serving or announcing it.
- Option b: it never subscribes and fetches only from nodes given by
  address, keeping nothing.
- Option c: it subscribes, and keeps nothing after a run.
- Recommendation: option a, because a have is 50 bytes per program and is
  the only way a thin node learns what exists without a new list operation,
  and a phone that runs a program twice should not download it twice.
  Built: `Node.Known` and `Node.Fetch` in `programnetwork/node.go`, the
  `thin` app kind on the phone.

D40 SuperNodeFullStorageDecision
- What exists now: #34 says a super node keeps everything until its storage
  is full, with no eviction.
- Question: what is full, and what does a full node take?
- Option a: full is the configured capacity (`-budget`) or, with none, a
  write that fails with no space left; a program that does not fit is
  refused and logged, a smaller one that still fits is taken, and the
  partial program of a failed write is dropped.
- Option b: the first refusal sets the node full, and it takes nothing more
  until a program is removed by hand.
- Option c: no capacity setting, only a full disk.
- Recommendation: option a, because a capacity keeps a super node from
  filling a disk it shares with other work, and taking a program that fits
  is not eviction and wastes no space. Built: `Node.onHave` and
  `Node.dropFull` in `programnetwork/node.go`, tested by
  `tests/program-network-super-full-no-eviction`.

D41 NodeTypeDefaultDecision
- What exists now: before #34 every node kept and forwarded every program,
  which is what a super node with no capacity does.
- Question: which type does `programnetwork node` run without `-type`?
- Option a: super, so every existing command, test and the flood app keep
  their behaviour; the phone's new thin app runs a thin node.
- Option b: medium with no budget and no pins, which keeps nothing until
  pins are given.
- Option c: thin, so a node keeps nothing unless told to.
- Recommendation: option a, because nothing changes for existing setups
  and a node that keeps nothing by default would end every flood at its
  first hop. Built: `NewNode` and the `-type` default.

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
    - ProgramTransportSmallPackets (#31): the short forms, block parts, get
      peer part and the packet size sweep. Depends on Client, Server and
      PeerGetPeers. Test: program-packet-size-sweep,
      program-transport-short-forms and
      program-transport-fetch-in-emulator-at-128-bytes.
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
    - ProgramNetworkNodeTypes (#34): thin, medium and super nodes, the pin
      list, the budget, persistence and `store remove`. Depends on
      FloodReplication. Test: program-network-node-types-mixed,
      program-network-super-full-no-eviction,
      program-network-node-restart-persistence and
      program-network-node-budget-accounting.
    - ProgramNetworkEmulatorThin (#34): the phone as a thin node fetches
      from a super node. Depends on NodeTypes. Test:
      program-network-thin-fetch-in-emulator.
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
