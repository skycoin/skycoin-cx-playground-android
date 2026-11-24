// Package objmodel holds the generational index of docs/ObjectModel.md. A Gid
// is an int64 with the slot index in bits 0 to 31, the generation in bits 32
// to 55 and the type tag in bits 56 to 63. Every check is a class B assert.
package objmodel

import (
	"fmt"

	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
)

// TypeTag names the object type of a Gid, 0x01 to 0x7F.
type TypeTag uint8

const (
	GidIndexBits      = 32
	GidGenerationBits = 24
	GidTypeTagBits    = 8

	GidIndexMask      uint64 = 0x0000_0000_FFFF_FFFF
	GidHighMask       uint64 = 0xFFFF_FFFF_0000_0000
	GidGenerationMask uint64 = 0x00FF_FFFF_0000_0000
	GidTypeTagMask    uint64 = 0xFF00_0000_0000_0000

	GidGenerationShift = 32
	GidTypeTagShift    = 56

	GidGenerationFirst uint32 = 1
	GidGenerationLast  uint32 = 0x00FF_FFFF

	GidTypeTagFirst TypeTag = 0x01
	GidTypeTagLast  TypeTag = 0x7F

	GidNil int64 = 0

	// GidCapMax is the largest Cap of a manager, so a slot count fits in uint32.
	GidCapMax uint64 = GidIndexMask
)

// GidCompose builds a Gid. It asserts the tag, generation and index ranges.
// VIOLATION STRHOT-228: E, error or assert text, built on a failure branch only, #412.
func GidCompose(typeTag TypeTag, generation uint32, index uint32) int64 {
	if typeTag < GidTypeTagFirst || typeTag > GidTypeTagLast {
		assert.AssertPreconditionTrue(false, fmt.Sprintf("Gid type tag %#x outside %#x to %#x", typeTag, GidTypeTagFirst, GidTypeTagLast))
	}
	if generation < GidGenerationFirst || generation >= GidGenerationLast {
		assert.AssertPreconditionTrue(false, fmt.Sprintf("Gid generation %#x outside %#x to %#x", generation, GidGenerationFirst, GidGenerationLast-1))
	}
	return int64(uint64(typeTag)<<GidTypeTagShift | uint64(generation)<<GidGenerationShift | uint64(index))
}

// GidIndex returns bits 0 to 31.
func GidIndex(gid int64) uint32 {
	return uint32(uint64(gid) & GidIndexMask)
}

// GidGeneration returns bits 32 to 55, the type tag masked out.
func GidGeneration(gid int64) uint32 {
	return uint32((uint64(gid) & GidGenerationMask) >> GidGenerationShift)
}

// GidTypeTag returns bits 56 to 63.
func GidTypeTag(gid int64) TypeTag {
	return TypeTag((uint64(gid) & GidTypeTagMask) >> GidTypeTagShift)
}

// GidAssertFailure is the cold path of a manager's Get and Release. It asserts
// the first failed check of gid and returns when gid is positive, carries
// typeTag and indexes a slot below capacity, so the manager then reports a
// slot that is not live or a stale generation.
// VIOLATION STRHOT-273: E, error or assert text, built on a failure branch only, #412.
//
//go:noinline
func GidAssertFailure(gid int64, typeTag TypeTag, capacity int) {
	if gid <= GidNil {
		assert.AssertPreconditionTrue(false, fmt.Sprintf("%s Gid %#x is nil or negative", GidTypeTagNameTable[typeTag], gid))
	}
	if GidTypeTag(gid) != typeTag {
		assert.AssertPreconditionTrue(false,
			fmt.Sprintf("%s Gid %#x has type tag %#x, want %#x", GidTypeTagNameTable[typeTag], gid, GidTypeTag(gid), typeTag))
	}
	if uint64(GidIndex(gid)) >= uint64(capacity) {
		assert.AssertPreconditionTrue(false,
			fmt.Sprintf("%s Gid %#x has index %d, Cap is %d", GidTypeTagNameTable[typeTag], gid, GidIndex(gid), capacity))
	}
}
