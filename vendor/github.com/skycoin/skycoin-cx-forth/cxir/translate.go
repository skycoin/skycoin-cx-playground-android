package cxir

import (
	"errors"
	"strconv"
	"strings"
)

// WordName is the Forth word a CX function compiles to.
func WordName(fn string) string { return "CX-" + fn }

// Translate turns p into Forth source. Each CX function becomes one colon
// definition with a frame of locals and a numbered label per IR line.
// Running the word CX-main runs the program.
// Each struct becomes a VM struct type whose id is the constant
// StructWordName(name), and a struct variable holds a StructRef.
//
// It checks types as it goes. The operands of an operator share one type
// that the operator takes, the output has the type it gives, a jump tests a
// Bool or Ternary, and a call matches the function it calls.
func Translate(p *Program) (string, error) {
	order, err := callOrder(p)
	if err != nil {
		return "", err
	}
	index, err := programIndexBuild(p)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i := range p.Structs {
		structDefinitionWrite(&b, &p.Structs[i])
	}
	for _, f := range order {
		if err := translateFunc(&b, f, index); err != nil {
			return "", errors.New(f.Name + ": " + err.Error())
		}
	}
	return b.String(), nil
}

// callOrder puts callees before callers. Self recursion is fine, mutual
// recursion is not supported.
func callOrder(p *Program) ([]*Function, error) {
	byName := map[string]*Function{}
	for i := range p.Functions {
		byName[p.Functions[i].Name] = &p.Functions[i]
	}
	if byName["main"] == nil {
		return nil, errors.New("no main function")
	}
	state := map[string]int{}
	var out []*Function
	var visit func(f *Function) error
	visit = func(f *Function) error {
		switch state[f.Name] {
		case 1:
			return errors.New("mutual recursion through " + f.Name + " is not supported")
		case 2:
			return nil
		}
		state[f.Name] = 1
		for _, e := range f.Exprs {
			if e.Op != OpCall || e.Call == f.Name {
				continue
			}
			g := byName[e.Call]
			if g == nil {
				return errors.New(f.Name + " calls undefined function " + e.Call)
			}
			if err := visit(g); err != nil {
				return err
			}
		}
		state[f.Name] = 2
		out = append(out, f)
		return nil
	}
	for i := range p.Functions {
		if err := visit(&p.Functions[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type fnCtx struct {
	b           *strings.Builder
	slots       map[string]int
	types       map[string]Type
	index       *programIndex
	structNames map[string]string // the struct of each struct variable
	n           int
}

// translateFunc writes f. With index nil, calls are not type checked and
// struct variables are not allowed.
func translateFunc(b *strings.Builder, f *Function, index *programIndex) error {
	c := &fnCtx{b: b, slots: map[string]int{}, types: map[string]Type{}, index: index, structNames: map[string]string{}, n: len(f.Exprs)}
	vars := make([]Arg, 0, len(f.Inputs)+len(f.Outputs)+len(f.Locals))
	vars = append(vars, f.Inputs...)
	vars = append(vars, f.Outputs...)
	vars = append(vars, f.Locals...)
	for i, v := range vars {
		if _, dup := c.slots[v.Name]; dup || v.Name == "" || strings.Contains(v.Name, ".") {
			return errors.New("bad or duplicate variable " + strconv.Quote(v.Name))
		}
		if _, ok := v.Type.AtomicType(); !ok {
			return errors.New("variable " + v.Name + " has no type")
		}
		if v.Type == TypeStruct {
			if c.index.structDefinition(v.structName()) == nil {
				return errors.New("variable " + v.Name + " has undefined struct " + strconv.Quote(v.structName()))
			}
			c.structNames[v.Name] = v.structName()
		}
		c.slots[v.Name] = i
		c.types[v.Name] = v.Type
	}

	// A function that makes struct instances and returns none frees them on
	// return, from the struct space top it keeps in an extra slot.
	release := -1
	for _, v := range vars[len(f.Inputs):] {
		if v.Type == TypeStruct {
			release = len(vars)
		}
	}
	for _, v := range f.Outputs {
		if v.Type == TypeStruct {
			release = -1
		}
	}
	frame := len(vars)
	if release >= 0 {
		frame++
	}

	c.w(": ", WordName(f.Name), "\n  ", strconv.Itoa(frame), " FRAME")
	for i := len(f.Inputs) - 1; i >= 0; i-- {
		c.w(" ", strconv.Itoa(i), " L!")
	}
	if release >= 0 {
		c.w(" StructSpaceTop ", strconv.Itoa(release), " L!")
	}
	for _, v := range vars[len(f.Inputs):] {
		if v.Type == TypeStruct {
			c.w(" ", StructWordName(v.structName()), " StructInstanceNew ", strconv.Itoa(c.slots[v.Name]), " L!")
			continue
		}
		if z := zero(v.Type); z != "" {
			c.w(" ", z, " ", strconv.Itoa(c.slots[v.Name]), " L!")
		}
	}
	c.w("\n")
	for i, e := range f.Exprs {
		c.w("  LABEL ", strconv.Itoa(i), " ")
		if err := c.expr(i, &e); err != nil {
			return errors.New("line " + strconv.Itoa(i) + ": " + err.Error())
		}
		c.w("\n")
	}
	c.w("  LABEL ", strconv.Itoa(c.n))
	if release >= 0 {
		c.w(" ", strconv.Itoa(release), " L@ StructSpaceRelease")
	}
	for _, v := range f.Outputs {
		c.w(" ", strconv.Itoa(c.slots[v.Name]), " L@")
	}
	c.w(" UNFRAME ;\n")
	return nil
}

func zero(t Type) string {
	switch t {
	case F32:
		return "0.0 >F32"
	case F64:
		return "0.0"
	case STR:
		return `S" "`
	case BOOL:
		return "FALSE"
	case TRI:
		return "NULL"
	}
	return ""
}

func (c *fnCtx) w(s ...string) {
	for _, x := range s {
		c.b.WriteString(x)
	}
}

func (c *fnCtx) push(a Arg) error {
	if !a.Lit {
		if _, err := c.typeOfArg(a); err != nil {
			return err
		}
		if a.structField() != "" {
			_, t, _ := c.structFieldOrdinal(a)
			c.structFieldSeek(a)
			c.w("StructCursorFieldGet ", structFieldLoadConvert(t))
			return nil
		}
		c.w(strconv.Itoa(c.slots[a.Name]), " L@ ")
		return nil
	}
	if a.Null {
		c.w("NULL ")
		return nil
	}
	switch a.Type {
	case I32, I64:
		c.w(strconv.FormatInt(a.I, 10), " ")
	case F32, F64:
		s := strconv.FormatFloat(a.F, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eIN") {
			s += ".0"
		}
		c.w(s, " ")
		if a.Type == F32 {
			c.w(">F32 ")
		}
	case BOOL, TRI:
		if a.B {
			c.w("TRUE ")
		} else {
			c.w("FALSE ")
		}
	case STR:
		c.str(a.S)
	default:
		return errors.New("literal of unknown type")
	}
	return nil
}

func (c *fnCtx) store(a Arg) error {
	if a.Lit {
		return errors.New("cannot assign to a literal")
	}
	if _, err := c.typeOfArg(a); err != nil {
		return err
	}
	if a.structField() != "" {
		_, t, _ := c.structFieldOrdinal(a)
		c.w(structFieldStoreConvert(t))
		c.structFieldSeek(a)
		c.w("StructCursorFieldSet ")
		return nil
	}
	c.w(strconv.Itoa(c.slots[a.Name]), " L! ")
	return nil
}

func narrow(t Type) string {
	switch t {
	case I32:
		return ">I32 "
	case F32:
		return ">F32 "
	}
	return ""
}

var opWords = map[Op]string{
	OpEqual: "= ", OpUnequal: "<> ", OpLt: "< ", OpGt: "> ", OpLteq: "<= ", OpGteq: ">= ",
	OpMul: "* ", OpDiv: "/ ", OpMod: "MOD ", OpAdd: "+ ", OpSub: "- ", OpNeg: "NEGATE ",
	OpBoolOr: "OR ", OpBoolAnd: "AND ", OpBoolNot: "NOT ", OpImplies: "IMPLIES ",
}

var arity = map[Op]int{
	OpIdentity: 1, OpNeg: 1, OpBoolNot: 1,
	OpEqual: 2, OpUnequal: 2, OpLt: 2, OpGt: 2, OpLteq: 2, OpGteq: 2,
	OpMul: 2, OpDiv: 2, OpMod: 2, OpAdd: 2, OpSub: 2,
	OpBoolOr: 2, OpBoolAnd: 2, OpImplies: 2,
}

func (c *fnCtx) target(line, off int) (string, error) {
	t := line + 1 + off
	if t < 0 || t > c.n {
		return "", errors.New("jump target out of range")
	}
	return strconv.Itoa(t), nil
}

func (c *fnCtx) expr(line int, e *Expr) error {
	switch e.Op {
	case OpJmp:
		if len(e.Inputs) != 1 {
			return errors.New("jmp needs one condition")
		}
		if err := c.typeCheckJmp(e); err != nil {
			return err
		}
		t, err1 := c.target(line, e.ThenLines)
		f, err2 := c.target(line, e.ElseLines)
		n := f
		var err3 error
		if e.HasNull {
			n, err3 = c.target(line, e.NullLines)
		}
		if err := errors.Join(err1, err2, err3); err != nil {
			return err
		}
		if err := c.push(e.Inputs[0]); err != nil {
			return err
		}
		c.w("JMP3 ", t, " ", f, " ", n)
		return nil
	case OpGoto:
		t, err := c.target(line, e.ThenLines)
		if err != nil {
			return err
		}
		c.w("GOTO ", t)
		return nil
	case OpReturn:
		c.w("GOTO ", strconv.Itoa(c.n))
		return nil
	case OpPrint:
		if len(e.Inputs) != 1 {
			return errors.New("print needs one input")
		}
		if err := c.push(e.Inputs[0]); err != nil {
			return err
		}
		c.w("PRINTLN")
		return nil
	case OpCall, OpHost:
		if err := c.typeCheckCall(e); err != nil {
			return err
		}
		for _, a := range e.Inputs {
			if err := c.push(a); err != nil {
				return err
			}
		}
		w := e.Call
		if e.Op == OpCall {
			w = WordName(w)
		} else if w == "" || strings.ContainsAny(w, " \t\n\r") {
			return errors.New("bad host word " + strconv.Quote(w))
		}
		c.w(w, " ")
		for i := len(e.Outputs) - 1; i >= 0; i-- {
			if err := c.store(e.Outputs[i]); err != nil {
				return err
			}
		}
		return nil
	}
	n, ok := arity[e.Op]
	if !ok {
		return errors.New("unsupported operator " + strconv.Itoa(int(e.Op)))
	}
	if len(e.Inputs) != n || len(e.Outputs) != 1 {
		return errors.New("wrong number of operands")
	}
	if err := c.typeCheckOperator(e); err != nil {
		return err
	}
	for _, a := range e.Inputs {
		if err := c.push(a); err != nil {
			return err
		}
	}
	c.w(opWords[e.Op], narrow(e.Outputs[0].Type))
	return c.store(e.Outputs[0])
}

// str pushes a string literal. Text with a quote, backslash or control
// character goes through S\" and its escapes.
func (c *fnCtx) str(s string) {
	if !strings.ContainsAny(s, "\"\\\n\r\t") {
		c.w(`S" `, s, `" `)
		return
	}
	b := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; ch {
		case '"', '\\':
			b = append(b, '\\', ch)
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			b = append(b, ch)
		}
	}
	c.w(`S\" `, string(b), `" `)
}
