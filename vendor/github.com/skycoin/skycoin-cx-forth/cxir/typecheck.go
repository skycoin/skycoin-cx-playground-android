package cxir

import (
	"errors"
	"strings"

	"github.com/skycoin/skycoin-cx-forth/forth"
)

// AtomicType returns the VM atomic type that values of t have, and false
// for a type the VM does not know.
func (t Type) AtomicType() (forth.AtomicType, bool) {
	switch t {
	case BOOL:
		return forth.AtomicTypeBool, true
	case I32:
		return forth.AtomicTypeInt32, true
	case I64:
		return forth.AtomicTypeInt64, true
	case F32:
		return forth.AtomicTypeFloat32, true
	case F64:
		return forth.AtomicTypeFloat64, true
	case STR:
		return forth.AtomicTypeString, true
	case TRI:
		return forth.AtomicTypeTernary, true
	case TypeStruct:
		return forth.AtomicTypeStructRef, true
	}
	return 0, false
}

// TypeName returns the atomic type name of t, such as Int32.
func TypeName(t Type) string {
	if a, ok := t.AtomicType(); ok {
		return forth.AtomicTypeName(a)
	}
	return "unknown type"
}

func typeNumeric(t Type) bool { return t == I32 || t == I64 || t == F32 || t == F64 }

// typeOfArg returns the type of a, checking a variable against its
// declaration.
func (c *fnCtx) typeOfArg(a Arg) (Type, error) {
	if a.Lit {
		if a.Type == TypeStruct {
			return 0, errors.New("a struct cannot be a literal")
		}
		return a.Type, nil
	}
	if a.structField() != "" {
		_, t, err := c.structFieldOrdinal(a)
		if err != nil {
			return 0, err
		}
		if a.Type != 0 && a.Type != t {
			return 0, errors.New("type error: " + a.Name + " is " + TypeName(t) + ", used as " + TypeName(a.Type))
		}
		return t, nil
	}
	t, ok := c.types[a.Name]
	if !ok {
		return 0, errors.New("undefined variable " + a.Name)
	}
	if a.Type != 0 && a.Type != t {
		return 0, errors.New("type error: " + a.Name + " is " + TypeName(t) + ", used as " + TypeName(a.Type))
	}
	return t, nil
}

// typeCheckOperator checks an operator line: the operands share one type
// the operator takes and the output has the type it gives.
func (c *fnCtx) typeCheckOperator(e *Expr) error {
	in := make([]Type, len(e.Inputs))
	for i, a := range e.Inputs {
		t, err := c.typeOfArg(a)
		if err != nil {
			return err
		}
		in[i] = t
	}
	out, err := c.typeOfArg(e.Outputs[0])
	if err != nil {
		return err
	}
	for _, x := range append(in, out) {
		if x == TypeStruct {
			return errors.New("type error: operators take no whole struct, use its fields")
		}
	}
	t := in[0]
	same := true
	for _, x := range in[1:] {
		same = same && x == t
	}
	bad := func(want string) error {
		names := make([]string, len(in))
		for i, x := range in {
			names[i] = TypeName(x)
		}
		return errors.New("type error: " + strings.Join(names, ", ") + " into " + TypeName(out) + ", want " + want)
	}
	switch e.Op {
	case OpIdentity:
		if t != out {
			return bad("the same type")
		}
	case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpNeg:
		if !same || out != t || !(typeNumeric(t) || (t == STR && e.Op == OpAdd)) {
			return bad("one number type, or String for add")
		}
	case OpEqual, OpUnequal, OpLt, OpGt, OpLteq, OpGteq:
		switch {
		case !same:
			return bad("one type")
		case out != BOOL && out != TRI:
			return bad("a Bool or Ternary result")
		case t == TRI && out != TRI:
			return bad("a Ternary result")
		case t == BOOL && e.Op != OpEqual && e.Op != OpUnequal:
			return bad("equal or unequal on Bool")
		}
	case OpBoolAnd, OpBoolOr, OpBoolNot, OpImplies:
		tri := false
		for _, x := range in {
			if x != BOOL && x != TRI {
				return bad("Bool or Ternary")
			}
			tri = tri || x == TRI
		}
		if (tri || e.Op == OpImplies) && out != TRI {
			return bad("a Ternary result")
		}
		if out != BOOL && out != TRI {
			return bad("a Bool or Ternary result")
		}
	}
	return nil
}

// typeCheckJmp checks that a jump condition is a Bool or Ternary.
func (c *fnCtx) typeCheckJmp(e *Expr) error {
	t, err := c.typeOfArg(e.Inputs[0])
	if err != nil {
		return err
	}
	if t != BOOL && t != TRI {
		return errors.New("type error: jmp on " + TypeName(t) + ", want Bool or Ternary")
	}
	return nil
}

// typeCheckCall matches a call against the function it calls. Host words
// declare no types, so their calls are not checked.
func (c *fnCtx) typeCheckCall(e *Expr) error {
	f := c.index.function(e.Call)
	if e.Op != OpCall || f == nil {
		return nil
	}
	if len(e.Inputs) != len(f.Inputs) || len(e.Outputs) != len(f.Outputs) {
		return errors.New("call of " + e.Call + " has the wrong number of arguments or results")
	}
	check := func(args, params []Arg) error {
		for i, a := range args {
			t, err := c.typeOfArg(a)
			if err != nil {
				return err
			}
			if t != params[i].Type || (t == TypeStruct && a.structField() == "" && c.structNames[a.Name] != params[i].structName()) {
				return errors.New("type error: " + e.Call + " takes " + TypeName(params[i].Type) +
					" for " + params[i].Name + ", got " + TypeName(t))
			}
		}
		return nil
	}
	return errors.Join(check(e.Inputs, f.Inputs), check(e.Outputs, f.Outputs))
}
