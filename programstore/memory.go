package programstore

import (
	"fmt"
	"slices"
	"sync"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// LoadedProgram is a program held in RAM: its verified files and the app
// manifest built from their cxapp.json, ready for apprun.Run.
type LoadedProgram struct {
	Address programblocks.Hash
	App     *manifest.Manifest
	Files   map[string][]byte
	Bytes   int64
}

// Memory holds programs in RAM, loaded from a store. It keeps at most
// Limit of them, dropping the one loaded first, so an ESP32 keeps one and a
// phone many. It is safe for concurrent use.
type Memory struct {
	Store *Store
	Limit int
	mu    sync.Mutex
	held  []*LoadedProgram // in load order
}

// NewMemory returns a Memory over s that keeps at most limit programs, or
// any number when limit is 0.
func NewMemory(s *Store, limit int) *Memory { return &Memory{Store: s, Limit: limit} }

// Load returns the program at addr, reading and verifying it from the
// store unless it is held already.
func (m *Memory) Load(addr programblocks.Hash) (*LoadedProgram, error) {
	if p := m.Held(addr); p != nil {
		return p, nil
	}
	files, err := m.Store.Files(addr)
	if err != nil {
		return nil, err
	}
	app, err := programblocks.AppManifest(files)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", addr, err)
	}
	p := &LoadedProgram{Address: addr, App: app, Files: files}
	for _, b := range files {
		p.Bytes += int64(len(b))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.heldLocked(addr); p != nil {
		return p, nil
	}
	if m.Limit > 0 && len(m.held) >= m.Limit {
		m.held = slices.Delete(m.held, 0, len(m.held)-m.Limit+1)
	}
	m.held = append(m.held, p)
	return p, nil
}

// Held returns the program at addr when it is in RAM, nil otherwise.
func (m *Memory) Held(addr programblocks.Hash) *LoadedProgram {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.heldLocked(addr)
}

func (m *Memory) heldLocked(addr programblocks.Hash) *LoadedProgram {
	for _, p := range m.held {
		if p.Address == addr {
			return p
		}
	}
	return nil
}

// Unload drops the program at addr from RAM.
func (m *Memory) Unload(addr programblocks.Hash) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.held = slices.DeleteFunc(m.held, func(p *LoadedProgram) bool { return p.Address == addr })
}

// Clear drops every program from RAM.
func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.held = nil
}

// HeldBytes returns the number of file bytes held in RAM.
func (m *Memory) HeldBytes() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, p := range m.held {
		n += p.Bytes
	}
	return n
}
