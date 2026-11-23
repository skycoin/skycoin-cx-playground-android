package forth

// Define adds a word that the host implements. fn pops its arguments and
// pushes its results with the Push and Pop helpers. A later Define of the
// same name shadows the earlier one, like any redefinition.
func (vm *VM) Define(name string, fn func(*VM)) {
	if i := vm.addWord(name, -1); i >= 0 {
		vm.words[i].Host = fn
	}
}

// Fail stops the running program with an error.
func (vm *VM) Fail(msg string) { vm.fail(msg) }

// PushString pushes a new string cell.
func (vm *VM) PushString(s string) { vm.Push(vm.intern(s)) }

// PopString pops a string cell.
func (vm *VM) PopString() string {
	c := vm.Pop()
	if c.T != TStr {
		if vm.err == nil {
			vm.fail("type error: want a string")
		}
		return ""
	}
	return vm.str(c)
}

// PopInt pops a number and truncates it to an integer.
func (vm *VM) PopInt() int64 {
	c := vm.Pop()
	switch {
	case c.IsFloat():
		return int64(c.Float())
	case c.T == TStr || c.T == TTri:
		if vm.err == nil {
			vm.fail("type error: want a number")
		}
		return 0
	}
	return c.V
}

// Strings returns the size of the string pool.
func (vm *VM) Strings() int { return len(vm.strs) }

// collect compacts the string pool. Every string cell lives in a stack, a
// local, data memory or the literal pool, so only those are scanned. It runs
// between instructions, where no primitive holds a cell in a Go variable.
// Cells copied out of the VM, for example by Stack, go stale after it runs.
func (vm *VM) collect() {
	live := make([]int64, len(vm.strs))
	for i := range live {
		live[i] = -1
	}
	roots := [][]Cell{vm.ds[:vm.sp], vm.rs[:vm.rp], vm.ls[:vm.lp], vm.mem[:vm.dp], vm.lits, vm.registers[:], vm.structCells[:vm.structTop]}
	for _, r := range roots {
		for _, c := range r {
			if c.T == TStr && c.V >= 0 && int(c.V) < len(live) {
				live[c.V] = 0
			}
		}
	}
	n := int64(0)
	vm.strSize = 0
	for i, l := range live {
		if l == 0 {
			live[i] = n
			vm.strs[n] = vm.strs[i]
			vm.strSize += len(vm.strs[n])
			n++
		}
	}
	for i := int(n); i < len(vm.strs); i++ {
		vm.strs[i] = ""
	}
	vm.strs = vm.strs[:n]
	for _, r := range roots {
		for i := range r {
			if c := &r[i]; c.T == TStr && c.V >= 0 && int(c.V) < len(live) {
				c.V = live[c.V]
			}
		}
	}
	vm.gcAt = max(StringsGC, 2*int(n))
	vm.gcSize = max(StringsGCBytes, 2*vm.strSize)
}

// primSQuoteEsc is S\" from Forth 2012 with the escapes \" \\ \n \r \t.
func primSQuoteEsc(vm *VM) {
	if vm.pos < len(vm.src) && vm.src[vm.pos] == ' ' {
		vm.pos++
	}
	b := make([]byte, 0, 32)
	for vm.pos < len(vm.src) && vm.src[vm.pos] != '"' {
		c := vm.src[vm.pos]
		vm.pos++
		if c == '\\' && vm.pos < len(vm.src) {
			c = vm.src[vm.pos]
			vm.pos++
			switch c {
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			case '"', '\\':
			default:
				vm.fail("bad escape in S\\\"")
				return
			}
		}
		b = append(b, c)
	}
	if vm.pos < len(vm.src) {
		vm.pos++
	}
	s := vm.intern(string(b))
	if vm.compiling {
		vm.emitLit(s)
		return
	}
	vm.Push(s)
}

// Usage reports how much of the VM's fixed space a program takes.
type Usage struct {
	Code, Words, Literals, Strings int
	StringBytes                    int
	CodeMax                        int
}

// Usage returns the space used so far.
func (vm *VM) Usage() Usage {
	return Usage{int(vm.here), len(vm.words), len(vm.lits), len(vm.strs), vm.strSize, len(vm.code)}
}
