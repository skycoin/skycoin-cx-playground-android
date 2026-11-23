package forth

import (
	"io"
	"strconv"
	"strings"
)

// Default sizes, chosen to fit comfortably in ESP32 DRAM.
const (
	StackSize  = 256
	RStackSize = 256
	LocalsSize = 512
	FramesSize = 128
	MemSize    = 2048
	CodeSize   = 4096
	WordsMax   = 512
	StringsGC  = 256 // string pool length that first triggers a compaction
	// StringsGCBytes is the string pool size that first triggers a compaction.
	StringsGCBytes = 16 << 10
)

// Error is a VM error. It is a plain type so no fmt is needed.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

const (
	opExit uint8 = iota
	opLit
	opCall
	opPrim
	opBranch
	opBranch0 // pop, jump unless true (false and null both jump)
	opBranch3 // pop, true falls through, false jumps to A, null jumps to next slot's A
	opSlot    // data slot after opBranch3, never executed
	opDo
	opLoop
	opPlusLoop
	opHost // A is the dictionary index of a host word
)

// Instr is one threaded code instruction.
type Instr struct {
	Op uint8
	A  int32
}

// Word is a dictionary entry.
type Word struct {
	Name string
	Prim int16 // primitive index, or -1 for a colon definition
	Imm  bool
	XT   int32
	Host func(*VM) // set for words the host defines
}

type ctl struct {
	kind   uint8
	a, b   int32
	fix    [2]int32
	nfix   int
	elsed  bool
	nulled bool
}

const (
	kIf uint8 = iota
	kTri
	kBegin
	kWhile
	kDo
)

type labelFix struct{ pos, label int32 }

// VM is a Forth machine with tagged cells.
type VM struct {
	ds    []Cell
	sp    int
	rs    []Cell
	rp    int
	ls    []Cell
	lp    int
	fs    []int32
	fp    int
	frame int32
	mem   []Cell
	dp    int64
	code  []Instr
	here  int32
	words []Word
	lits  []Cell
	strs  []string
	out   io.Writer
	obuf  []byte
	base  int
	err   error

	src       string
	pos       int
	compiling bool
	cur       int // dictionary index of the word being defined
	cstack    []ctl
	labels    map[int32]int32
	lfix      []labelFix
	saveHere  int32

	gcAt    int // string pool length that triggers the next compaction
	strSize int // bytes held by the string pool
	gcSize  int // strSize that triggers the next compaction

	registers     [RegisterCount]Cell
	registerFocus uint8

	structCells        []Cell
	structTop          int
	structTypes        []structTypeEntry
	structDefineOpen   bool
	structDefineFields []AtomicType

	structCursorRef     Cell
	structCursorOrdinal int32
}

// Sizes sets the fixed space of a VM, in cells for the stacks, locals, data
// memory and struct space and in instructions for code. A zero field takes the default.
type Sizes struct {
	Stack, RStack, Locals, Frames, Mem, Code, StructCells int
}

// New returns a VM with the default sizes writing output to out.
func New(out io.Writer) *VM { return NewSized(out, Sizes{}) }

// NewSized returns a VM with the sizes in s.
func NewSized(out io.Writer, s Sizes) *VM {
	or := func(n, def int) int {
		if n > 0 {
			return n
		}
		return def
	}
	vm := &VM{out: out, base: 10, words: make([]Word, 0, 128), obuf: make([]byte, 0, 64), gcAt: StringsGC, gcSize: StringsGCBytes,
		ds:          make([]Cell, or(s.Stack, StackSize)),
		rs:          make([]Cell, or(s.RStack, RStackSize)),
		ls:          make([]Cell, or(s.Locals, LocalsSize)),
		fs:          make([]int32, or(s.Frames, FramesSize)),
		mem:         make([]Cell, or(s.Mem, MemSize)),
		code:        make([]Instr, or(s.Code, CodeSize)),
		structCells: make([]Cell, or(s.StructCells, StructCellsSize)),
	}
	for i := range prims {
		vm.words = append(vm.words, Word{Name: prims[i].name, Prim: int16(i), Imm: prims[i].imm})
	}
	vm.structTypeBuiltInDefine()
	return vm
}

func (vm *VM) fail(msg string) {
	if vm.err == nil {
		vm.err = &Error{msg}
	}
}

func (vm *VM) failWord(msg, w string) { vm.fail(msg + ": " + w) }

// Push pushes a cell onto the data stack.
func (vm *VM) Push(c Cell) {
	if vm.sp >= len(vm.ds) {
		vm.fail("stack overflow")
		return
	}
	vm.ds[vm.sp] = c
	vm.sp++
}

// Pop pops a cell from the data stack.
func (vm *VM) Pop() Cell {
	if vm.sp <= 0 {
		vm.fail("stack underflow")
		return Cell{}
	}
	vm.sp--
	return vm.ds[vm.sp]
}

// Depth returns the number of cells on the data stack.
func (vm *VM) Depth() int { return vm.sp }

// Stack returns a copy of the data stack, bottom first.
func (vm *VM) Stack() []Cell { return append([]Cell(nil), vm.ds[:vm.sp]...) }

func (vm *VM) rpush(c Cell) {
	if vm.rp >= len(vm.rs) {
		vm.fail("return stack overflow")
		return
	}
	vm.rs[vm.rp] = c
	vm.rp++
}

func (vm *VM) rpop() Cell {
	if vm.rp <= 0 {
		vm.fail("return stack underflow")
		return Cell{}
	}
	vm.rp--
	return vm.rs[vm.rp]
}

func (vm *VM) write(b []byte) {
	if vm.out != nil {
		_, _ = vm.out.Write(b)
	}
}

func (vm *VM) writeString(s string) {
	vm.obuf = append(vm.obuf[:0], s...)
	vm.write(vm.obuf)
}

func (vm *VM) intern(s string) Cell {
	vm.strSize += len(s)
	vm.strs = append(vm.strs, s)
	return Cell{TStr, int64(len(vm.strs) - 1)}
}

func (vm *VM) str(c Cell) string {
	if c.V < 0 || int(c.V) >= len(vm.strs) {
		return ""
	}
	return vm.strs[c.V]
}

// String returns the text of a string cell.
func (vm *VM) String(c Cell) string { return vm.str(c) }

func (vm *VM) emit(op uint8, a int32) int32 {
	if int(vm.here) >= len(vm.code) {
		vm.fail("code space full")
		return vm.here
	}
	p := vm.here
	vm.code[p] = Instr{op, a}
	vm.here++
	return p
}

func (vm *VM) emitLit(c Cell) {
	vm.lits = append(vm.lits, c)
	vm.emit(opLit, int32(len(vm.lits)-1))
}

func (vm *VM) find(name string) int {
	for i := len(vm.words) - 1; i >= 0; i-- {
		if strings.EqualFold(vm.words[i].Name, name) {
			return i
		}
	}
	return -1
}

func (vm *VM) addWord(name string, xt int32) int {
	if len(vm.words) >= WordsMax {
		vm.fail("dictionary full")
		return -1
	}
	vm.words = append(vm.words, Word{Name: strings.Clone(name), Prim: -1, XT: xt})
	return len(vm.words) - 1
}

// run executes threaded code starting at xt until it returns.
func (vm *VM) run(xt int32) {
	base := vm.rp
	vm.rpush(Int(-1))
	ip := xt
	for vm.err == nil {
		if ip < 0 || ip >= vm.here {
			vm.fail("bad instruction pointer")
			break
		}
		if len(vm.strs) >= vm.gcAt || vm.strSize >= vm.gcSize {
			vm.collect()
		}
		in := vm.code[ip]
		ip++
		switch in.Op {
		case opExit:
			ip = int32(vm.rpop().V)
			if ip < 0 {
				return
			}
		case opLit:
			vm.Push(vm.lits[in.A])
		case opCall:
			vm.rpush(Int(int64(ip)))
			ip = in.A
		case opPrim:
			prims[in.A].fn(vm)
		case opHost:
			vm.words[in.A].Host(vm)
		case opBranch:
			ip = in.A
		case opBranch0:
			if vm.Pop().Truth() != True {
				ip = in.A
			}
		case opBranch3:
			switch vm.Pop().Truth() {
			case True:
				ip++
			case False:
				ip = in.A
			default:
				ip = vm.code[ip].A
			}
		case opDo:
			start := vm.Pop()
			limit := vm.Pop()
			vm.rpush(limit)
			vm.rpush(start)
		case opLoop, opPlusLoop:
			if vm.rp < 2 {
				vm.fail("loop without DO")
				break
			}
			step := int64(1)
			if in.Op == opPlusLoop {
				step = vm.Pop().V
			}
			idx := vm.rs[vm.rp-1].V + step
			limit := vm.rs[vm.rp-2].V
			if (step >= 0 && idx < limit) || (step < 0 && idx >= limit) {
				vm.rs[vm.rp-1].V = idx
				ip = in.A
			} else {
				vm.rp -= 2
			}
		default:
			vm.fail("bad opcode")
		}
	}
	vm.rp = base
}

func (vm *VM) execute(i int) {
	w := &vm.words[i]
	if w.Host != nil {
		w.Host(vm)
		return
	}
	if w.Prim >= 0 {
		prims[w.Prim].fn(vm)
		return
	}
	vm.run(w.XT)
}

func (vm *VM) next() string {
	s := vm.src
	for vm.pos < len(s) && s[vm.pos] <= ' ' {
		vm.pos++
	}
	st := vm.pos
	for vm.pos < len(s) && s[vm.pos] > ' ' {
		vm.pos++
	}
	return s[st:vm.pos]
}

// parseTo reads raw text up to delim and skips the delimiter.
func (vm *VM) parseTo(delim byte) string {
	if vm.pos < len(vm.src) && vm.src[vm.pos] == ' ' {
		vm.pos++
	}
	st := vm.pos
	for vm.pos < len(vm.src) && vm.src[vm.pos] != delim {
		vm.pos++
	}
	s := vm.src[st:vm.pos]
	if vm.pos < len(vm.src) {
		vm.pos++
	}
	return s
}

func (vm *VM) number(tok string) (Cell, bool) {
	if n, err := strconv.ParseInt(tok, vm.base, 64); err == nil {
		return Int(n), true
	}
	if vm.base == 10 && strings.ContainsAny(tok, ".eE") {
		if f, err := strconv.ParseFloat(tok, 64); err == nil {
			return F64(f), true
		}
	}
	return Cell{}, false
}

// Interpret runs Forth source text. Compilation state carries over
// between calls, so a definition may span several lines.
func (vm *VM) Interpret(src string) error {
	vm.src, vm.pos, vm.err = src, 0, nil
	for vm.err == nil {
		if len(vm.strs) >= vm.gcAt || vm.strSize >= vm.gcSize {
			vm.collect()
		}
		tok := vm.next()
		if tok == "" {
			break
		}
		i := vm.find(tok)
		if i >= 0 {
			w := vm.words[i]
			if vm.compiling && !w.Imm {
				if w.Host != nil {
					vm.emit(opHost, int32(i))
				} else if w.Prim >= 0 {
					vm.emit(opPrim, int32(w.Prim))
				} else {
					vm.emit(opCall, w.XT)
				}
			} else {
				vm.execute(i)
			}
			continue
		}
		c, ok := vm.number(tok)
		if !ok {
			vm.failWord("unknown word", tok)
			break
		}
		if vm.compiling {
			vm.emitLit(c)
		} else {
			vm.Push(c)
		}
	}
	if err := vm.err; err != nil {
		vm.reset()
		return err
	}
	return nil
}

func (vm *VM) reset() {
	vm.sp, vm.rp, vm.lp, vm.fp, vm.frame = 0, 0, 0, 0, 0
	if vm.compiling {
		vm.words = vm.words[:vm.cur]
		vm.here = vm.saveHere
		vm.compiling = false
	}
	vm.structDefineOpen = false
	vm.cstack = vm.cstack[:0]
}

// Run interprets src and returns everything written while doing so.
// It is a convenience for tests and the CX translator.
func Run(src string) (string, error) {
	var b strings.Builder
	vm := New(&b)
	err := vm.Interpret(src)
	return b.String(), err
}

// Mark records the dictionary, code, data and literal pools, like MARKER.
type Mark struct {
	words, lits, strs, structTypes, structTop int
	here                                      int32
	dp                                        int64
}

// Mark returns the current allocation state.
func (vm *VM) Mark() Mark {
	return Mark{len(vm.words), len(vm.lits), len(vm.strs), len(vm.structTypes), vm.structTop, vm.here, vm.dp}
}

// Restore forgets everything defined since m was taken.
func (vm *VM) Restore(m Mark) {
	vm.words = vm.words[:m.words]
	vm.lits = vm.lits[:m.lits]
	vm.strs = vm.strs[:min(m.strs, len(vm.strs))]
	vm.here, vm.dp = m.here, m.dp
	vm.structCursorClose(m.structTop)
	vm.structTypes = vm.structTypes[:m.structTypes]
	vm.structTop = m.structTop
	vm.reset()
}
