package ast

import (
	"fmt"

	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

// SourceFileGid names one source file path in SourceFileMng, see #413. It has
// the Gid layout of docs/ObjectModel.md, and every slot keeps
// GidGenerationFirst, since a file is never released. SourceFileGidNil is no
// file, the empty path.
type SourceFileGid int64

const (
	SourceFileGidTypeTag objmodel.TypeTag = 0x10
	SourceFileGidNil     SourceFileGid    = 0
)

// SourceFileMng holds the path of every source file once, as a string pool
// Gid. It is written by hand like the string pool, because a file is never
// released.
type SourceFileMng struct {
	SourceFileMngPaths []stringpool.StringGid
}

// SourceFileMngInit sets up an empty manager with room for capacity files.
func (manager *SourceFileMng) SourceFileMngInit(capacity int) {
	manager.SourceFileMngPaths = make([]stringpool.StringGid, 0, capacity)
}

// SourceFileIntern returns the Gid of the file with path, registering it on
// first use. A program has few files, so a scan finds an earlier one.
func (manager *SourceFileMng) SourceFileIntern(path stringpool.StringGid) SourceFileGid {
	if path == stringpool.StringGidNil {
		return SourceFileGidNil
	}
	for slot, have := range manager.SourceFileMngPaths {
		if have == path {
			return SourceFileGid(objmodel.GidCompose(SourceFileGidTypeTag, objmodel.GidGenerationFirst, uint32(slot)))
		}
	}
	slot := len(manager.SourceFileMngPaths)
	if uint64(slot) >= objmodel.GidCapMax {
		assert.AssertInvariantTrue(false, fmt.Sprintf("SourceFileMng full at %d files", slot))
	}
	manager.SourceFileMngPaths = append(manager.SourceFileMngPaths, path)
	return SourceFileGid(objmodel.GidCompose(SourceFileGidTypeTag, objmodel.GidGenerationFirst, uint32(slot)))
}

// SourceFilePath returns the path of the file gid names. SourceFileGidNil
// gives StringGidNil.
// VIOLATION STRHOT-313: E, error or assert text, built on a failure branch only, #412.
func (manager *SourceFileMng) SourceFilePath(gid SourceFileGid) stringpool.StringGid {
	if gid == SourceFileGidNil {
		return stringpool.StringGidNil
	}
	slot := objmodel.GidIndex(int64(gid))
	if objmodel.GidTypeTag(int64(gid)) != SourceFileGidTypeTag || objmodel.GidGeneration(int64(gid)) != objmodel.GidGenerationFirst || uint64(slot) >= uint64(len(manager.SourceFileMngPaths)) {
		objmodel.GidAssertFailure(int64(gid), SourceFileGidTypeTag, len(manager.SourceFileMngPaths))
		assert.AssertPreconditionTrue(false, fmt.Sprintf("SourceFile Gid %#x has generation %#x, want %#x", int64(gid), objmodel.GidGeneration(int64(gid)), objmodel.GidGenerationFirst))
	}
	return manager.SourceFileMngPaths[slot]
}

// SourceFileLen returns the number of registered files.
func (manager *SourceFileMng) SourceFileLen() int {
	return len(manager.SourceFileMngPaths)
}
