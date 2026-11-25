package ast

import (
	"slices"
	"strings"

	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

// NativeNameIndex lists the names of built-ins, opcodes and library functions
// or constants, with their codes, see #413. The compiler finds a built-in by
// the name Gids of its parts through CXProgramNativeFind, so it builds no
// `pkg + "." + name` key. Entries are sorted by name once looked up, which
// keeps the names of a package in one range.
type NativeNameIndex struct {
	NativeNameIndexEntries []NativeNameEntry
	NativeNameIndexSorted  bool
	// NativeNameIndexGeneration changes whenever an entry is added or the
	// index is reset, which invalidates the lookups programs cached.
	NativeNameIndexGeneration uint32
	// NativeNameIndexLinkedOnly hides the entries NativeLinked does not report.
	NativeNameIndexLinkedOnly bool
}

// NativeNameEntry is one registered built-in name and its code.
type NativeNameEntry struct {
	NativeNameEntryText string
	NativeNameEntryCode int
	// NativeNameEntryBytes is the text as bytes, for printing without a string.
	NativeNameEntryBytes []byte
}

// NativeNameKey is the name a program looks a built-in up by, the Gids of its
// package, receiver and name. An empty part is left out of the name, so
// {os, nil, ReadFile} is "os.ReadFile" and {nil, nil, len} is "len".
type NativeNameKey struct {
	NativeNameKeyIndex    *NativeNameIndex
	NativeNameKeyPackage  stringpool.StringGid
	NativeNameKeyReceiver stringpool.StringGid
	NativeNameKeyName     stringpool.StringGid
}

// NativeNameFound is a cached lookup, the entry found or none, and the
// generation of the index it was found in.
type NativeNameFound struct {
	NativeNameFoundEntry      NativeNameEntry
	NativeNameFoundOk         bool
	NativeNameFoundGeneration uint32
}

// NativeNames holds the names of constants.NativeNameTable, the opcodes and
// the library functions. A lookup finds only those NativeLinked reports.
// VIOLATION: init chaining. Init 2: built from the table at package init.
var NativeNames = nativeNamesMake()

func nativeNamesMake() NativeNameIndex {
	entries := make([]NativeNameEntry, len(constants.NativeNameTable))
	for i, name := range constants.NativeNameTable {
		text := name.NativeNameText
		entries[i] = NativeNameEntry{NativeNameEntryText: text, NativeNameEntryCode: name.NativeNameCode, NativeNameEntryBytes: []byte(text)[:len(text):len(text)]}
	}
	return NativeNameIndex{NativeNameIndexEntries: entries, NativeNameIndexSorted: true, NativeNameIndexLinkedOnly: true}
}

// NativeLinked reports whether this binary links the built-in code, an opcode,
// a library function key or an opcode of NativeFunctionHost.
func NativeLinked(code int) bool {
	if code >= 0 {
		return constants.OpcodeLinked(code) || NativeFunctionHost != nil && NativeFunctionHost(nil, code) != nil
	}
	library, function := constants.LibraryFunctionKeyParts(code)
	return LibraryFunctionLinked(library, function)
}

// NativeNameIndexAdd registers text as the name of code. Registering a name
// again replaces its code.
func (index *NativeNameIndex) NativeNameIndexAdd(code int, text string) {
	if index.NativeNameIndexSorted {
		position, found := slices.BinarySearchFunc(index.NativeNameIndexEntries, text, nativeNameEntryCompareText)
		if found {
			if index.NativeNameIndexEntries[position].NativeNameEntryCode != code {
				index.NativeNameIndexEntries[position].NativeNameEntryCode = code
				index.NativeNameIndexGeneration++
			}
			return
		}
	}
	index.NativeNameIndexEntries = append(index.NativeNameIndexEntries, NativeNameEntry{NativeNameEntryText: text, NativeNameEntryCode: code, NativeNameEntryBytes: []byte(text)[:len(text):len(text)]})
	index.NativeNameIndexSorted = false
	index.NativeNameIndexGeneration++
}

// NativeNameIndexChanged invalidates the lookups programs cached, for when
// what NativeLinked reports changes.
func (index *NativeNameIndex) NativeNameIndexChanged() {
	index.NativeNameIndexGeneration++
}

// NativeNameIndexReset removes every entry.
func (index *NativeNameIndex) NativeNameIndexReset() {
	index.NativeNameIndexEntries = nil
	index.NativeNameIndexSorted = false
	index.NativeNameIndexGeneration++
}

// NativeNameIndexAll returns every entry in name order. The caller must not
// change them.
func (index *NativeNameIndex) NativeNameIndexAll() []NativeNameEntry {
	index.nativeNameIndexSort()
	return index.NativeNameIndexEntries
}

// NativeNameIndexLen returns the number of names.
func (index *NativeNameIndex) NativeNameIndexLen() int {
	index.nativeNameIndexSort()
	return len(index.NativeNameIndexEntries)
}

// nativeNameIndexSort sorts the entries by name. Of a name added twice since
// the last sort, the later code stays.
func (index *NativeNameIndex) nativeNameIndexSort() {
	if index.NativeNameIndexSorted {
		return
	}
	entries := index.NativeNameIndexEntries
	slices.SortStableFunc(entries, func(left NativeNameEntry, right NativeNameEntry) int {
		return strings.Compare(left.NativeNameEntryText, right.NativeNameEntryText)
	})
	kept := entries[:0]
	for position, entry := range entries {
		if position+1 < len(entries) && entries[position+1].NativeNameEntryText == entry.NativeNameEntryText {
			continue
		}
		kept = append(kept, entry)
	}
	index.NativeNameIndexEntries = kept
	index.NativeNameIndexSorted = true
}

// NativeNameIndexText returns the entry named text, for tools and tests that
// name a built-in in Go source.
func (index *NativeNameIndex) NativeNameIndexText(text string) (NativeNameEntry, bool) {
	index.nativeNameIndexSort()
	position, found := slices.BinarySearchFunc(index.NativeNameIndexEntries, text, nativeNameEntryCompareText)
	if !found || (index.NativeNameIndexLinkedOnly && !NativeLinked(index.NativeNameIndexEntries[position].NativeNameEntryCode)) {
		return NativeNameEntry{}, false
	}
	return index.NativeNameIndexEntries[position], true
}

// NativeNameIndexCodeText returns the name registered for code, or "" when
// none is. It scans every entry, so only diagnostics and printing call it.
func (index *NativeNameIndex) NativeNameIndexCodeText(code int) string {
	for _, entry := range index.NativeNameIndexEntries {
		if entry.NativeNameEntryCode == code {
			return entry.NativeNameEntryText
		}
	}
	return ""
}

// NativeNameIndexCodeBytes returns the bytes of NativeNameIndexCodeText
// without building a string, nil when no name is. The caller must not write
// to them.
func (index *NativeNameIndex) NativeNameIndexCodeBytes(code int) []byte {
	for i := range index.NativeNameIndexEntries {
		entry := &index.NativeNameIndexEntries[i]
		if entry.NativeNameEntryCode == code {
			return entry.NativeNameEntryBytes
		}
	}
	return nil
}

// NativeNameIndexPackage returns the entries whose name is "pkg." followed by
// a member name, in name order.
func (index *NativeNameIndex) NativeNameIndexPackage(pkg string) []NativeNameEntry {
	index.nativeNameIndexSort()
	entries := index.NativeNameIndexEntries
	first, _ := slices.BinarySearchFunc(entries, pkg, nativeNameEntryCompareText)
	end := first
	for end < len(entries) && strings.HasPrefix(entries[end].NativeNameEntryText, pkg) {
		end++
	}
	// Within the range, a name such as "pkg" or "pkgx.y" is no member.
	return slices.DeleteFunc(slices.Clone(entries[first:end]), func(entry NativeNameEntry) bool {
		return (index.NativeNameIndexLinkedOnly && !NativeLinked(entry.NativeNameEntryCode)) || len(entry.NativeNameEntryText) == len(pkg) || entry.NativeNameEntryText[len(pkg)] != '.'
	})
}

func nativeNameEntryCompareText(entry NativeNameEntry, text string) int {
	return strings.Compare(entry.NativeNameEntryText, text)
}

// nativeNameCompare compares text with the parts joined by dots, leaving out
// the empty parts, without building the joined name.
func nativeNameCompare(text string, parts *[3][]byte) int {
	position := 0
	first := true
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		if !first {
			if position == len(text) {
				return -1
			}
			if text[position] != '.' {
				return nativeNameByteCompare(text[position], '.')
			}
			position++
		}
		first = false
		for _, partByte := range part {
			if position == len(text) {
				return -1
			}
			if text[position] != partByte {
				return nativeNameByteCompare(text[position], partByte)
			}
			position++
		}
	}
	if position < len(text) {
		return 1
	}
	return 0
}

func nativeNameByteCompare(left byte, right byte) int {
	if left < right {
		return -1
	}
	return 1
}

// nativeNameIndexFind looks the name parts up, comparing their bytes.
func (index *NativeNameIndex) nativeNameIndexFind(parts *[3][]byte) (NativeNameEntry, bool) {
	index.nativeNameIndexSort()
	// A loop instead of slices.BinarySearchFunc, whose callback makes parts
	// escape to the heap.
	entries := index.NativeNameIndexEntries
	low, high := 0, len(entries)
	for low < high {
		middle := int(uint(low+high) >> 1)
		order := nativeNameCompare(entries[middle].NativeNameEntryText, parts)
		if order == 0 {
			return entries[middle], true
		}
		if order < 0 {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return NativeNameEntry{}, false
}

// CXProgramNativeFind returns the built-in of index named by the Gids pkg,
// receiver and name, any of which may be StringGidNil. A name is compared by
// its bytes once, then the program finds it again by its Gids.
func (prgrm *CXProgram) CXProgramNativeFind(index *NativeNameIndex, pkg stringpool.StringGid, receiver stringpool.StringGid, name stringpool.StringGid) (NativeNameEntry, bool) {
	state := prgrm.CxIrState
	key := NativeNameKey{NativeNameKeyIndex: index, NativeNameKeyPackage: pkg, NativeNameKeyReceiver: receiver, NativeNameKeyName: name}
	if cached, found := state.CxIrStateNativeNames[key]; found && cached.NativeNameFoundGeneration == index.NativeNameIndexGeneration {
		return cached.NativeNameFoundEntry, cached.NativeNameFoundOk
	}
	parts := [3][]byte{prgrm.CXProgramNameBytes(pkg), prgrm.CXProgramNameBytes(receiver), prgrm.CXProgramNameBytes(name)}
	entry, found := index.nativeNameIndexFind(&parts)
	if found && index.NativeNameIndexLinkedOnly && !NativeLinked(entry.NativeNameEntryCode) {
		entry, found = NativeNameEntry{}, false
	}
	if state.CxIrStateNativeNames == nil {
		state.CxIrStateNativeNames = make(map[NativeNameKey]NativeNameFound)
	}
	state.CxIrStateNativeNames[key] = NativeNameFound{NativeNameFoundEntry: entry, NativeNameFoundOk: found, NativeNameFoundGeneration: index.NativeNameIndexGeneration}
	return entry, found
}
