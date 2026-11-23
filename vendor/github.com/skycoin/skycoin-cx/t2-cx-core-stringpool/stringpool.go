// Package stringpool interns the names of the compiler IR, see #413 and
// docs/ObjectModel.md. A StringGid names one interned byte string, and equal
// strings have equal Gids, so comparing two names is comparing two integers.
package stringpool

import (
	"bytes"
	"fmt"
	"unsafe"

	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
)

// StringGid names one interned string. It has the Gid layout of
// docs/ObjectModel.md. Every slot keeps GidGenerationFirst, since an interned
// string is never released. StringGidNil is the empty string.
type StringGid int64

const (
	StringGidTypeTag objmodel.TypeTag = 0x0F
	StringGidNil     StringGid        = 0
)

// StringPool holds every interned string in one byte arena, with the offset
// and length of each slot. No Go string is stored. StringPoolIndex finds the
// slot of a string while compiling and is nil once the pool is frozen.
type StringPool struct {
	StringPoolArena   []byte
	StringPoolOffsets []uint32
	StringPoolLengths []uint32
	StringPoolIndex   map[string]StringGid
	StringPoolFrozen  bool
}

// StringPoolInit sets up an empty pool with room for slotCap strings and
// arenaCap bytes before the first growth.
func (pool *StringPool) StringPoolInit(slotCap int, arenaCap int) {
	pool.StringPoolArena = make([]byte, 0, arenaCap)
	pool.StringPoolOffsets = make([]uint32, 0, slotCap)
	pool.StringPoolLengths = make([]uint32, 0, slotCap)
	pool.StringPoolIndex = make(map[string]StringGid, slotCap)
	pool.StringPoolFrozen = false
}

// StringPoolIntern returns the Gid of bytes, interning them on first use.
// The empty string is StringGidNil. It asserts on a frozen pool.
func (pool *StringPool) StringPoolIntern(bytes []byte) StringGid {
	if pool.StringPoolFrozen {
		assert.AssertPreconditionTrue(false, fmt.Sprintf("StringPoolIntern of %q on a frozen pool", string(bytes)))
	}
	if len(bytes) == 0 {
		return StringGidNil
	}
	if gid, ok := pool.StringPoolIndex[string(bytes)]; ok {
		return gid
	}
	slot := len(pool.StringPoolOffsets)
	if uint64(slot) >= objmodel.GidCapMax || uint64(len(pool.StringPoolArena))+uint64(len(bytes)) > uint64(^uint32(0)) {
		assert.AssertInvariantTrue(false, fmt.Sprintf("StringPool full at %d strings and %d bytes", slot, len(pool.StringPoolArena)))
	}
	offset := len(pool.StringPoolArena)
	pool.StringPoolArena = append(pool.StringPoolArena, bytes...)
	pool.StringPoolOffsets = append(pool.StringPoolOffsets, uint32(offset))
	pool.StringPoolLengths = append(pool.StringPoolLengths, uint32(len(bytes)))
	gid := StringGid(objmodel.GidCompose(StringGidTypeTag, objmodel.GidGenerationFirst, uint32(slot)))
	// The key views the arena. Arena bytes are never written again, and an
	// array left behind by a growing append stays alive while the map holds it.
	pool.StringPoolIndex[unsafe.String(&pool.StringPoolArena[offset], len(bytes))] = gid
	return gid
}

// StringPoolFind returns the Gid of bytes without interning them, and false
// when they were never interned. On a frozen pool it scans every slot, so it is
// for rare run time lookups by text such as affordances.
func (pool *StringPool) StringPoolFind(text []byte) (StringGid, bool) {
	if len(text) == 0 {
		return StringGidNil, true
	}
	if pool.StringPoolIndex != nil {
		gid, found := pool.StringPoolIndex[string(text)]
		return gid, found
	}
	for slot, offset := range pool.StringPoolOffsets {
		if bytes.Equal(pool.StringPoolArena[offset:offset+pool.StringPoolLengths[slot]], text) {
			return StringGid(objmodel.GidCompose(StringGidTypeTag, objmodel.GidGenerationFirst, uint32(slot))), true
		}
	}
	return StringGidNil, false
}

// StringPoolFreeze ends interning. It is called when compilation ends, and the
// VM runs only on a frozen pool.
func (pool *StringPool) StringPoolFreeze() {
	pool.StringPoolIndex = nil
	pool.StringPoolFrozen = true
}

// StringPoolBytes returns a view into the arena of the string gid names. The
// caller must not write to it. StringGidNil gives an empty slice.
func (pool *StringPool) StringPoolBytes(gid StringGid) []byte {
	if gid == StringGidNil {
		return nil
	}
	slot := objmodel.GidIndex(int64(gid))
	if objmodel.GidTypeTag(int64(gid)) != StringGidTypeTag || objmodel.GidGeneration(int64(gid)) != objmodel.GidGenerationFirst || uint64(slot) >= uint64(len(pool.StringPoolOffsets)) {
		objmodel.GidAssertFailure(int64(gid), StringGidTypeTag, len(pool.StringPoolOffsets))
		assert.AssertPreconditionTrue(false, fmt.Sprintf("String Gid %#x has generation %#x, want %#x", int64(gid), objmodel.GidGeneration(int64(gid)), objmodel.GidGenerationFirst))
	}
	offset := pool.StringPoolOffsets[slot]
	return pool.StringPoolArena[offset : offset+pool.StringPoolLengths[slot] : offset+pool.StringPoolLengths[slot]]
}

// StringPoolEqual reports whether a and b name equal strings.
func StringPoolEqual(a StringGid, b StringGid) bool {
	return a == b
}

// StringPoolLen returns the number of interned strings.
func (pool *StringPool) StringPoolLen() int {
	return len(pool.StringPoolOffsets)
}

// StringPoolText returns the string gid names as a Go string that views the
// arena, so it does not allocate. Arena bytes are never written again, which
// keeps the view valid for as long as it is held. It is for boundary code
// that still takes strings, such as diagnostics and the string keyed maps.
func (pool *StringPool) StringPoolText(gid StringGid) string {
	bytes := pool.StringPoolBytes(gid)
	if len(bytes) == 0 {
		return ""
	}
	return unsafe.String(&bytes[0], len(bytes))
}
