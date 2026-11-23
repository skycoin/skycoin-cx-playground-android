package forth

import (
	"strconv"
	"strings"
)

// Immediate words. They run while compiling and lay down threaded code.
var compilePrims = []prim{
	{":", primColon, false},
	{";", primSemi, true},
	{"EXIT", func(vm *VM) { vm.needCompile(); vm.emit(opExit, 0) }, true},
	{"[']", primBracketTick, true},
	{"RECURSE", func(vm *VM) { vm.needCompile(); vm.emit(opCall, vm.words[vm.cur].XT) }, true},
	{"(", func(vm *VM) { vm.parseTo(')') }, true},
	{"\\", func(vm *VM) { vm.parseTo('\n') }, true},
	{".\"", primDotQuote, true},
	{"S\"", primSQuote, true},
	{"S\\\"", primSQuoteEsc, true},

	{"IF", func(vm *VM) {
		vm.needCompile()
		vm.cpush(ctl{kind: kIf, a: vm.emit(opBranch0, -1)})
	}, true},
	{"TRI-IF", func(vm *VM) {
		vm.needCompile()
		p := vm.emit(opBranch3, -1)
		vm.emit(opSlot, -1)
		vm.cpush(ctl{kind: kTri, a: p})
	}, true},
	{"ELSE", primElse, true},
	{"NULL-ELSE", primNullElse, true},
	{"THEN", primThen, true},

	{"BEGIN", func(vm *VM) { vm.needCompile(); vm.cpush(ctl{kind: kBegin, a: vm.here}) }, true},
	{"UNTIL", func(vm *VM) {
		if c, ok := vm.cpop(kBegin); ok {
			vm.emit(opBranch0, c.a)
		}
	}, true},
	{"AGAIN", func(vm *VM) {
		if c, ok := vm.cpop(kBegin); ok {
			vm.emit(opBranch, c.a)
		}
	}, true},
	{"WHILE", func(vm *VM) {
		if c, ok := vm.cpop(kBegin); ok {
			c.kind = kWhile
			c.b = vm.emit(opBranch0, -1)
			vm.cpush(c)
		}
	}, true},
	{"REPEAT", func(vm *VM) {
		if c, ok := vm.cpop(kWhile); ok {
			vm.emit(opBranch, c.a)
			vm.code[c.b].A = vm.here
		}
	}, true},
	{"DO", func(vm *VM) {
		vm.needCompile()
		vm.emit(opDo, 0)
		vm.cpush(ctl{kind: kDo, a: vm.here})
	}, true},
	{"LOOP", func(vm *VM) {
		if c, ok := vm.cpop(kDo); ok {
			vm.emit(opLoop, c.a)
		}
	}, true},
	{"+LOOP", func(vm *VM) {
		if c, ok := vm.cpop(kDo); ok {
			vm.emit(opPlusLoop, c.a)
		}
	}, true},

	// Numbered labels inside one definition, for translating jump based IR.
	{"LABEL", func(vm *VM) {
		vm.needCompile()
		if n, ok := vm.labelArg(); ok {
			vm.labels[n] = vm.here
		}
	}, true},
	{"GOTO", func(vm *VM) {
		vm.needCompile()
		if n, ok := vm.labelArg(); ok {
			vm.lfix = append(vm.lfix, labelFix{vm.emit(opBranch, -1), n})
		}
	}, true},
	// JMP3 t f n ( cond -- ) jumps to label t, f or n by the ternary value.
	{"JMP3", func(vm *VM) {
		vm.needCompile()
		t, ok1 := vm.labelArg()
		f, ok2 := vm.labelArg()
		n, ok3 := vm.labelArg()
		if !ok1 || !ok2 || !ok3 {
			return
		}
		p := vm.emit(opBranch3, -1)
		vm.emit(opSlot, -1)
		q := vm.emit(opBranch, -1)
		vm.lfix = append(vm.lfix, labelFix{p, f}, labelFix{p + 1, n}, labelFix{q, t})
	}, true},
}

func (vm *VM) needCompile() {
	if !vm.compiling {
		vm.fail("compile only word")
	}
}

func (vm *VM) cpush(c ctl) {
	if vm.err == nil {
		vm.cstack = append(vm.cstack, c)
	}
}

func (vm *VM) cpop(kind uint8) (ctl, bool) {
	vm.needCompile()
	n := len(vm.cstack)
	if vm.err != nil || n == 0 || vm.cstack[n-1].kind != kind {
		vm.fail("unbalanced control structure")
		return ctl{}, false
	}
	c := vm.cstack[n-1]
	vm.cstack = vm.cstack[:n-1]
	return c, true
}

func (vm *VM) top() *ctl {
	n := len(vm.cstack)
	if !vm.compiling || n == 0 {
		vm.fail("unbalanced control structure")
		return nil
	}
	return &vm.cstack[n-1]
}

func (vm *VM) labelArg() (int32, bool) {
	tok := vm.next()
	n, err := strconv.ParseInt(tok, 10, 32)
	if err != nil {
		vm.failWord("bad label", tok)
		return 0, false
	}
	return int32(n), true
}

func primColon(vm *VM) {
	if vm.compiling {
		vm.fail("nested definition")
		return
	}
	name := vm.next()
	if name == "" {
		vm.fail("missing name")
		return
	}
	vm.saveHere = vm.here
	vm.cur = len(vm.words)
	// The word is visible during its own definition so CX functions can recurse.
	if vm.addWord(name, vm.here) < 0 {
		return
	}
	vm.compiling = true
	vm.cstack = vm.cstack[:0]
	vm.labels = map[int32]int32{}
	vm.lfix = vm.lfix[:0]
}

func primSemi(vm *VM) {
	vm.needCompile()
	if len(vm.cstack) != 0 {
		vm.fail("unbalanced control structure")
		return
	}
	vm.emit(opExit, 0)
	for _, f := range vm.lfix {
		t, ok := vm.labels[f.label]
		if !ok {
			vm.failWord("undefined label", strconv.Itoa(int(f.label)))
			return
		}
		vm.code[f.pos].A = t
	}
	vm.compiling = false
}

func primElse(vm *VM) {
	c := vm.top()
	if c == nil {
		return
	}
	switch {
	case c.kind == kIf:
		p := vm.emit(opBranch, -1)
		vm.code[c.a].A = vm.here
		c.a = p
	case c.kind == kTri && !c.elsed:
		c.fix[c.nfix] = vm.emit(opBranch, -1)
		c.nfix++
		c.elsed = true
		vm.code[c.a].A = vm.here
	default:
		vm.fail("ELSE without IF")
	}
}

func primNullElse(vm *VM) {
	c := vm.top()
	if c == nil {
		return
	}
	if c.kind != kTri || c.nulled {
		vm.fail("NULL-ELSE without TRI-IF")
		return
	}
	c.fix[c.nfix] = vm.emit(opBranch, -1)
	c.nfix++
	c.nulled = true
	vm.code[c.a+1].A = vm.here
}

// primThen closes IF or TRI-IF. In a TRI-IF without NULL-ELSE, null
// takes the ELSE branch, the same as plain IF.
func primThen(vm *VM) {
	n := len(vm.cstack)
	if !vm.compiling || n == 0 {
		vm.fail("THEN without IF")
		return
	}
	c := vm.cstack[n-1]
	switch c.kind {
	case kIf:
		vm.code[c.a].A = vm.here
	case kTri:
		if !c.elsed {
			vm.code[c.a].A = vm.here
		}
		if !c.nulled {
			vm.code[c.a+1].A = vm.code[c.a].A
		}
		for i := 0; i < c.nfix; i++ {
			vm.code[c.fix[i]].A = vm.here
		}
	default:
		vm.fail("THEN without IF")
		return
	}
	vm.cstack = vm.cstack[:n-1]
}

func primDotQuote(vm *VM) {
	s := vm.parseTo('"')
	if vm.compiling {
		vm.emitLit(vm.intern(strings.Clone(s)))
		vm.emit(opPrim, vm.primIndex("TYPE"))
		return
	}
	vm.writeString(s)
}

func primSQuote(vm *VM) {
	s := vm.parseTo('"')
	if vm.compiling {
		vm.emitLit(vm.intern(strings.Clone(s)))
		return
	}
	vm.Push(vm.intern(s))
}

func (vm *VM) primIndex(name string) int32 {
	for i := range prims {
		if prims[i].name == name {
			return int32(i)
		}
	}
	return -1
}
