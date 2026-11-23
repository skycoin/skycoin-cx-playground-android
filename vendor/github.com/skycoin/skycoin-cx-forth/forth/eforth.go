package forth

// The eForth ternary encoding from ternary.fs is TRUE=-1 FALSE=0 MAYBE=1.
// These words reproduce its definitions exactly on integers and use Kleene
// logic on tagged ternary cells. See doc.go.

func eNot(v int64) int64 {
	if v == 1 {
		return 1
	}
	return ^v
}

func eEquiv(a, b int64) int64 { return a&b | eNot(a)&eNot(b) }

// eDecode reads an eForth ternary integer the way ternary.fs t. prints it.
func eDecode(v int64) int64 {
	switch v {
	case -1:
		return True
	case 0:
		return False
	}
	return Null
}

func (vm *VM) triOf(c Cell) int64 {
	switch c.T {
	case TTri:
		return c.V
	case TInt:
		return eDecode(c.V)
	}
	vm.fail("not a truth value")
	return Null
}

func primTNot(vm *VM) {
	c := vm.Pop()
	if c.T == TInt {
		vm.Push(Int(eNot(c.V)))
		return
	}
	vm.Push(Tri(TriNot(vm.triOf(c))))
}

// tlogic stays in the eForth integer encoding when both operands are
// integers and gives a ternary cell otherwise.
func (vm *VM) tlogic(op byte) {
	b, a := vm.Pop(), vm.Pop()
	if vm.err != nil {
		return
	}
	if a.T == TInt && b.T == TInt {
		var r int64
		switch op {
		case '&':
			r = a.V & b.V
		case '|':
			r = a.V | b.V
		case 'E':
			r = eEquiv(a.V, b.V)
		case '>':
			r = eNot(a.V) | b.V
		case '^':
			r = eNot(eEquiv(a.V, b.V))
		}
		vm.Push(Int(r))
		return
	}
	x, y := vm.triOf(a), vm.triOf(b)
	var r int64
	switch op {
	case '&':
		r = TriAnd(x, y)
	case '|':
		r = TriOr(x, y)
	case 'E':
		r = TriEqv(x, y)
	case '>':
		r = TriImplies(x, y)
	case '^':
		r = TriXor(x, y)
	}
	vm.Push(Tri(r))
}

func primTDot(vm *VM) {
	switch vm.triOf(vm.Pop()) {
	case True:
		vm.writeString("T")
	case False:
		vm.writeString("F")
	default:
		vm.writeString("?")
	}
}

func (vm *VM) tick() int {
	name := vm.next()
	i := vm.find(name)
	if i < 0 {
		vm.failWord("unknown word", name)
	}
	return i
}

func primExecute(vm *VM) {
	c := vm.Pop()
	if vm.err != nil {
		return
	}
	if c.T != TInt || c.V < 0 || c.V >= int64(len(vm.words)) {
		vm.fail("bad execution token")
		return
	}
	vm.execute(int(c.V))
}

func primBracketTick(vm *VM) {
	vm.needCompile()
	if i := vm.tick(); i >= 0 {
		vm.emitLit(Int(int64(i)))
	}
}
