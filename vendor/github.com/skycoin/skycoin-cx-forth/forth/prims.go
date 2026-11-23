package forth

import (
	"math"
	"strconv"
)

type prim struct {
	name string
	fn   func(*VM)
	imm  bool
}

var prims []prim

func init() {
	prims = []prim{
		// stack
		{"DUP", func(vm *VM) { c := vm.Pop(); vm.Push(c); vm.Push(c) }, false},
		{"DROP", func(vm *VM) { vm.Pop() }, false},
		{"SWAP", func(vm *VM) { b, a := vm.Pop(), vm.Pop(); vm.Push(b); vm.Push(a) }, false},
		{"OVER", func(vm *VM) { b, a := vm.Pop(), vm.Pop(); vm.Push(a); vm.Push(b); vm.Push(a) }, false},
		{"ROT", func(vm *VM) { c, b, a := vm.Pop(), vm.Pop(), vm.Pop(); vm.Push(b); vm.Push(c); vm.Push(a) }, false},
		{"-ROT", func(vm *VM) { c, b, a := vm.Pop(), vm.Pop(), vm.Pop(); vm.Push(c); vm.Push(a); vm.Push(b) }, false},
		{"NIP", func(vm *VM) { b := vm.Pop(); vm.Pop(); vm.Push(b) }, false},
		{"TUCK", func(vm *VM) { b, a := vm.Pop(), vm.Pop(); vm.Push(b); vm.Push(a); vm.Push(b) }, false},
		{"2DUP", func(vm *VM) { b, a := vm.Pop(), vm.Pop(); vm.Push(a); vm.Push(b); vm.Push(a); vm.Push(b) }, false},
		{"2DROP", func(vm *VM) { vm.Pop(); vm.Pop() }, false},
		{"PICK", primPick, false},
		{"DEPTH", func(vm *VM) { vm.Push(Int(int64(vm.sp))) }, false},
		{">R", func(vm *VM) { vm.rpush(vm.Pop()) }, false},
		{"R>", func(vm *VM) { vm.Push(vm.rpop()) }, false},
		{"R@", func(vm *VM) { c := vm.rpop(); vm.rpush(c); vm.Push(c) }, false},

		// arithmetic
		{"+", func(vm *VM) { vm.arith('+') }, false},
		{"-", func(vm *VM) { vm.arith('-') }, false},
		{"*", func(vm *VM) { vm.arith('*') }, false},
		{"/", func(vm *VM) { vm.arith('/') }, false},
		{"MOD", func(vm *VM) { vm.arith('%') }, false},
		{"MIN", func(vm *VM) { vm.arith('m') }, false},
		{"MAX", func(vm *VM) { vm.arith('M') }, false},
		{"/MOD", func(vm *VM) {
			b, a := vm.Pop(), vm.Pop()
			vm.Push(a)
			vm.Push(b)
			vm.arith('%')
			vm.Push(a)
			vm.Push(b)
			vm.arith('/')
		}, false},
		{"NEGATE", func(vm *VM) { vm.Push(Int(-1)); vm.arith('*') }, false},
		{"ABS", primAbs, false},
		{"1+", func(vm *VM) { vm.Push(Int(1)); vm.arith('+') }, false},
		{"1-", func(vm *VM) { vm.Push(Int(1)); vm.arith('-') }, false},
		{">I32", func(vm *VM) { c := vm.num(); vm.Push(nullOr(c, Int(int64(int32(c.V))))) }, false},
		{">INT", func(vm *VM) { c := vm.num(); vm.Push(nullOr(c, Int(c.V))) }, false},
		{">F32", func(vm *VM) { c := vm.Pop(); vm.Push(nullOr(c, F32(float32(c.Float())))) }, false},
		{">F64", func(vm *VM) { c := vm.Pop(); vm.Push(nullOr(c, F64(c.Float()))) }, false},

		// comparison, all return ternary cells
		{"=", func(vm *VM) { vm.compare('=') }, false},
		{"<>", func(vm *VM) { vm.compare('!') }, false},
		{"<", func(vm *VM) { vm.compare('<') }, false},
		{">", func(vm *VM) { vm.compare('>') }, false},
		{"<=", func(vm *VM) { vm.compare('l') }, false},
		{">=", func(vm *VM) { vm.compare('g') }, false},
		{"0=", func(vm *VM) { vm.Push(Int(0)); vm.compare('=') }, false},
		{"0<", func(vm *VM) { vm.Push(Int(0)); vm.compare('<') }, false},

		// logic: bitwise on two integers, Kleene otherwise
		{"AND", func(vm *VM) { vm.logic('&') }, false},
		{"OR", func(vm *VM) { vm.logic('|') }, false},
		{"XOR", func(vm *VM) { vm.logic('^') }, false},
		{"IMPLIES", func(vm *VM) { vm.logic('>') }, false},
		{"LIMPLIES", func(vm *VM) { vm.logic('L') }, false},
		{"EQV", func(vm *VM) { vm.logic('E') }, false},
		{"NOT", func(vm *VM) { vm.Push(Tri(TriNot(vm.Pop().Truth()))) }, false},
		{"INVERT", func(vm *VM) {
			c := vm.Pop()
			if c.T == TInt {
				vm.Push(Int(^c.V))
			} else {
				vm.Push(Tri(TriNot(c.Truth())))
			}
		}, false},

		// ternary
		{"TRUE", func(vm *VM) { vm.Push(Tri(True)) }, false},
		{"FALSE", func(vm *VM) { vm.Push(Tri(False)) }, false},
		{"NULL", func(vm *VM) { vm.Push(Tri(Null)) }, false},
		{"KNOWN?", func(vm *VM) { vm.Push(Bool(!vm.Pop().IsNull())) }, false},
		{"NULL?", func(vm *VM) { vm.Push(Bool(vm.Pop().IsNull())) }, false},
		{"TRUE?", func(vm *VM) { vm.Push(Bool(vm.Pop().Truth() == True)) }, false},
		{"FALSE?", func(vm *VM) { vm.Push(Bool(vm.Pop().Truth() == False)) }, false},
		{">TRI", func(vm *VM) { vm.Push(Tri(vm.Pop().Truth())) }, false},

		// eForth vocabulary from ternary.fs, see doc.go
		{"MAYBE", func(vm *VM) { vm.Push(Tri(Null)) }, false},
		{"TNOT", primTNot, false},
		{"TAND", func(vm *VM) { vm.tlogic('&') }, false},
		{"TOR", func(vm *VM) { vm.tlogic('|') }, false},
		{"TEQUIV", func(vm *VM) { vm.tlogic('E') }, false},
		{"TIMPLY", func(vm *VM) { vm.tlogic('>') }, false},
		{"TXOR", func(vm *VM) { vm.tlogic('^') }, false},
		{"T.", primTDot, false},

		// execution tokens are dictionary indices
		{"'", func(vm *VM) { vm.Push(Int(int64(vm.tick()))) }, false},
		{"EXECUTE", primExecute, false},

		// memory, addresses are cell indices
		{"@", func(vm *VM) {
			if a := vm.addr(); a >= 0 {
				vm.Push(vm.mem[a])
			}
		}, false},
		{"!", func(vm *VM) {
			if a := vm.addr(); a >= 0 {
				vm.mem[a] = vm.Pop()
			}
		}, false},
		{"+!", func(vm *VM) {
			if a := vm.addr(); a >= 0 {
				vm.Push(vm.mem[a])
				vm.arith('+')
				vm.mem[a] = vm.Pop()
			}
		}, false},
		{",", func(vm *VM) { vm.allot(1); vm.mem[vm.dp-1] = vm.Pop() }, false},
		{"HERE", func(vm *VM) { vm.Push(Int(vm.dp)) }, false},
		{"ALLOT", func(vm *VM) { vm.allot(vm.Pop().V) }, false},
		{"CELLS", func(vm *VM) {}, false},
		{"VARIABLE", func(vm *VM) { vm.create(Int(vm.dp)); vm.allot(1) }, false},
		{"CONSTANT", func(vm *VM) { vm.create(vm.Pop()) }, false},

		// output
		{".", func(vm *VM) { vm.print(vm.Pop(), ' ') }, false},
		{"PRINTLN", func(vm *VM) { vm.print(vm.Pop(), '\n') }, false},
		{"TYPE", func(vm *VM) { vm.print(vm.Pop(), 0) }, false},
		{">STR", func(vm *VM) { vm.Push(vm.intern(string(vm.appendCell(nil, vm.Pop())))) }, false},
		{"CR", func(vm *VM) { vm.writeString("\n") }, false},
		{"SPACE", func(vm *VM) { vm.writeString(" ") }, false},
		{"SPACES", func(vm *VM) {
			for n := vm.Pop().V; n > 0 && vm.err == nil; n-- {
				vm.writeString(" ")
			}
		}, false},
		{"PAGE", func(vm *VM) { vm.writeString("\x1b[2J\x1b[H") }, false},
		{"EMIT", func(vm *VM) { vm.obuf = append(vm.obuf[:0], byte(vm.Pop().V)); vm.write(vm.obuf) }, false},
		{".S", primDotS, false},
		{"DECIMAL", func(vm *VM) { vm.base = 10 }, false},
		{"HEX", func(vm *VM) { vm.base = 16 }, false},
		{"WORDS", primWords, false},

		// local frames, used by the CX translator
		{"FRAME", primFrame, false},
		{"UNFRAME", primUnframe, false},
		{"L@", func(vm *VM) {
			if i := vm.local(); i >= 0 {
				vm.Push(vm.ls[i])
			}
		}, false},
		{"L!", func(vm *VM) {
			if i := vm.local(); i >= 0 {
				vm.ls[i] = vm.Pop()
			}
		}, false},
		{"I", func(vm *VM) { vm.loopIndex(1) }, false},
		{"J", func(vm *VM) { vm.loopIndex(3) }, false},
	}
	prims = append(prims, atomicTypePrims()...)
	prims = append(prims, registerPrims()...)
	prims = append(prims, structPrims()...)
	prims = append(prims, structCursorPrims()...)
	prims = append(prims, compilePrims...)
}

func nullOr(c, v Cell) Cell {
	if c.IsNull() {
		return c
	}
	return v
}

func (vm *VM) num() Cell {
	c := vm.Pop()
	if c.IsFloat() {
		return Int(int64(c.Float()))
	}
	return c
}

func primPick(vm *VM) {
	n := int(vm.Pop().V)
	if n < 0 || n >= vm.sp {
		vm.fail("stack underflow")
		return
	}
	vm.Push(vm.ds[vm.sp-1-n])
}

func primAbs(vm *VM) {
	c := vm.Pop()
	switch {
	case c.IsFloat():
		f := math.Abs(c.Float())
		if c.T == TF32 {
			vm.Push(F32(float32(f)))
		} else {
			vm.Push(F64(f))
		}
	case c.T == TTri:
		vm.fail("arithmetic on a ternary value")
	case c.T == TInt && c.V < 0:
		vm.Push(Int(-c.V))
	default:
		vm.Push(c)
	}
}

// arith applies a binary arithmetic operator. Ternary operands, NULL
// included, are an error: a truth value is not a number. Integer operands
// must share a type, except that an Int64 takes the other operand's type,
// and the result wraps to that type's width.
func (vm *VM) arith(op byte) {
	b, a := vm.Pop(), vm.Pop()
	if vm.err != nil {
		return
	}
	if a.T == TStr && b.T == TStr && op == '+' {
		vm.Push(vm.intern(vm.str(a) + vm.str(b)))
		return
	}
	if a.T == TTri || b.T == TTri {
		vm.fail("arithmetic on a ternary value")
		return
	}
	if !atomicTypeNumeric(a.T) || !atomicTypeNumeric(b.T) {
		vm.fail("type error in arithmetic")
		return
	}
	if a.IsFloat() || b.IsFloat() {
		x, y := a.Float(), b.Float()
		var r float64
		switch op {
		case '+':
			r = x + y
		case '-':
			r = x - y
		case '*':
			r = x * y
		case '/':
			r = x / y
		case '%':
			r = math.Mod(x, y)
		case 'm':
			r = math.Min(x, y)
		case 'M':
			r = math.Max(x, y)
		}
		if a.T == TF64 || b.T == TF64 {
			vm.Push(F64(r))
		} else {
			vm.Push(F32(float32(r)))
		}
		return
	}
	t, ok := atomicTypeIntegerResult(a.T, b.T)
	if !ok {
		vm.fail("type error: " + AtomicTypeName(a.T) + " and " + AtomicTypeName(b.T) + " in arithmetic")
		return
	}
	x, y := a.V, b.V
	var r int64
	switch op {
	case '+':
		r = x + y
	case '-':
		r = x - y
	case '*':
		r = x * y
	case '/', '%':
		switch {
		case y == 0:
			vm.fail("division by zero")
			return
		case atomicTypeUnsigned(t) && op == '/':
			r = int64(uint64(x) / uint64(y))
		case atomicTypeUnsigned(t):
			r = int64(uint64(x) % uint64(y))
		case op == '/':
			r = x / y
		default:
			r = x % y
		}
	case 'm', 'M':
		r = x
		if c := atomicTypeCompareInteger(Cell{t, x}, Cell{t, y}); (c > 0) == (op == 'm') && c != 0 {
			r = y
		}
	}
	vm.Push(atomicTypeWrap(t, r))
}

// compare pushes a ternary result. Any null operand gives null.
func (vm *VM) compare(op byte) {
	b, a := vm.Pop(), vm.Pop()
	if vm.err != nil {
		return
	}
	if a.IsNull() || b.IsNull() {
		vm.Push(Tri(Null))
		return
	}
	var c int
	switch {
	case a.T == TStr && b.T == TStr:
		c = compareOrdered(vm.str(a), vm.str(b))
	case a.T == TTri || b.T == TTri, a.T == AtomicTypeBool || b.T == AtomicTypeBool,
		a.T == AtomicTypeStructRef || b.T == AtomicTypeStructRef:
		if a.T != b.T {
			vm.fail("type error in comparison")
			return
		}
		c = compareOrdered(a.V, b.V)
	case a.IsFloat() || b.IsFloat():
		c = compareOrdered(a.Float(), b.Float())
	default:
		c = atomicTypeCompareInteger(a, b)
	}
	var r bool
	switch op {
	case '=':
		r = c == 0
	case '!':
		r = c != 0
	case '<':
		r = c < 0
	case '>':
		r = c > 0
	case 'l':
		r = c <= 0
	case 'g':
		r = c >= 0
	}
	vm.Push(Bool(r))
}

func (vm *VM) logic(op byte) {
	b, a := vm.Pop(), vm.Pop()
	if vm.err != nil {
		return
	}
	if a.T == TInt && b.T == TInt {
		switch op {
		case '&':
			vm.Push(Int(a.V & b.V))
			return
		case '|':
			vm.Push(Int(a.V | b.V))
			return
		case '^':
			vm.Push(Int(a.V ^ b.V))
			return
		}
	}
	x, y := a.Truth(), b.Truth()
	var r int64
	switch op {
	case '&':
		r = TriAnd(x, y)
	case '|':
		r = TriOr(x, y)
	case '^':
		r = TriXor(x, y)
	case '>':
		r = TriImplies(x, y)
	case 'L':
		r = TriLukImplies(x, y)
	case 'E':
		r = TriEqv(x, y)
	}
	vm.Push(Tri(r))
}

func (vm *VM) addr() int64 {
	a := vm.Pop()
	if vm.err != nil {
		return -1
	}
	if a.T != TInt || a.V < 0 || a.V >= int64(len(vm.mem)) {
		vm.fail("bad address")
		return -1
	}
	return a.V
}

func (vm *VM) allot(n int64) {
	if vm.dp+n > int64(len(vm.mem)) || vm.dp+n < 0 {
		vm.fail("memory full")
		return
	}
	vm.dp += n
}

// create defines the next word as a constant pushing c.
func (vm *VM) create(c Cell) {
	name := vm.next()
	if name == "" {
		vm.fail("missing name")
		return
	}
	vm.addWord(name, vm.here)
	vm.emitLit(c)
	vm.emit(opExit, 0)
}

func (vm *VM) print(c Cell, sep byte) {
	vm.obuf = vm.appendCell(vm.obuf[:0], c)
	if sep != 0 {
		vm.obuf = append(vm.obuf, sep)
	}
	vm.write(vm.obuf)
}

func primDotS(vm *VM) {
	vm.obuf = append(vm.obuf[:0], '<')
	vm.obuf = strconv.AppendInt(vm.obuf, int64(vm.sp), 10)
	vm.obuf = append(vm.obuf, "> "...)
	for i := 0; i < vm.sp; i++ {
		vm.obuf = vm.appendCell(vm.obuf, vm.ds[i])
		vm.obuf = append(vm.obuf, ' ')
	}
	vm.write(vm.obuf)
}

func primWords(vm *VM) {
	for i := len(vm.words) - 1; i >= 0; i-- {
		vm.writeString(vm.words[i].Name)
		vm.writeString(" ")
	}
}

// primFrame ( n -- ) opens a frame of n zeroed locals.
func primFrame(vm *VM) {
	n := int(vm.Pop().V)
	if vm.fp >= len(vm.fs) || vm.lp+n > len(vm.ls) || n < 0 {
		vm.fail("locals overflow")
		return
	}
	vm.fs[vm.fp] = vm.frame
	vm.fp++
	vm.frame = int32(vm.lp)
	for i := 0; i < n; i++ {
		vm.ls[vm.lp+i] = Cell{}
	}
	vm.lp += n
}

func primUnframe(vm *VM) {
	if vm.fp <= 0 {
		vm.fail("frame underflow")
		return
	}
	vm.lp = int(vm.frame)
	vm.fp--
	vm.frame = vm.fs[vm.fp]
}

func (vm *VM) local() int {
	i := int(vm.frame) + int(vm.Pop().V)
	if vm.err != nil {
		return -1
	}
	if i < int(vm.frame) || i >= vm.lp {
		vm.fail("bad local")
		return -1
	}
	return i
}

func (vm *VM) loopIndex(off int) {
	if vm.rp < off {
		vm.fail("not in a loop")
		return
	}
	vm.Push(vm.rs[vm.rp-off])
}
