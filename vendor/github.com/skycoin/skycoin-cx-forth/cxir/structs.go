package cxir

import (
	"errors"
	"strconv"
	"strings"

	"github.com/skycoin/skycoin-cx-forth/forth"
)

// StructDefinition is a CX struct type, ~ cx/ast/ast_cxstruct.go CXStruct.
// Each field is an ordinal of the VM struct type, in order.
type StructDefinition struct {
	Name   string
	Fields []Arg
}

// A struct variable keeps its struct name in S, and a field of one is the
// variable name and the field name joined by a dot, as CX writes p.x. So a
// program without structs pays nothing in the size of its Args.

func (a Arg) structName() string {
	if a.Type != TypeStruct || a.Lit {
		return ""
	}
	return a.S
}

// structField returns the field of a field Arg, or "".
func (a Arg) structField() string {
	if i := strings.IndexByte(a.Name, '.'); i >= 0 && !a.Lit {
		return a.Name[i+1:]
	}
	return ""
}

// variable returns the name of the variable a refers to, without a field.
func (a Arg) variable() string {
	if i := strings.IndexByte(a.Name, '.'); i >= 0 {
		return a.Name[:i]
	}
	return a.Name
}

// StructWordName is the Forth constant that holds a CX struct's type id.
func StructWordName(name string) string { return "CxStruct" + name }

// VarStruct references a variable of struct type structName.
func VarStruct(name, structName string) Arg {
	return Arg{Name: name, Type: TypeStruct, S: structName}
}

// VarField references field of the struct variable name. t may be 0 to
// leave the field's type unchecked.
func VarField(name, field string, t Type) Arg {
	return Arg{Name: name + "." + field, Type: t}
}

// programIndex holds a program's functions and structs by name. A nil
// index checks no calls and knows no structs.
type programIndex struct {
	functions map[string]*Function
	structs   map[string]*StructDefinition
}

func (x *programIndex) function(name string) *Function {
	if x == nil {
		return nil
	}
	return x.functions[name]
}

func (x *programIndex) structDefinition(name string) *StructDefinition {
	if x == nil {
		return nil
	}
	return x.structs[name]
}

// programIndexBuild indexes p and checks its struct definitions.
func programIndexBuild(p *Program) (*programIndex, error) {
	x := &programIndex{
		functions: make(map[string]*Function, len(p.Functions)),
		structs:   make(map[string]*StructDefinition, len(p.Structs)),
	}
	for i := range p.Functions {
		x.functions[p.Functions[i].Name] = &p.Functions[i]
	}
	for i := range p.Structs {
		s := &p.Structs[i]
		if s.Name == "" || strings.ContainsAny(s.Name, " \t\n\r") || x.structs[s.Name] != nil {
			return nil, errors.New("bad or duplicate struct " + strconv.Quote(s.Name))
		}
		if len(s.Fields) == 0 {
			return nil, errors.New("struct " + s.Name + " has no fields")
		}
		seen := map[string]bool{}
		for _, f := range s.Fields {
			if f.Name == "" || seen[f.Name] {
				return nil, errors.New("struct " + s.Name + ": bad or duplicate field " + strconv.Quote(f.Name))
			}
			seen[f.Name] = true
			if f.Type == TypeStruct {
				return nil, errors.New("struct " + s.Name + ": field " + f.Name + " is a struct, which is not supported yet")
			}
			if _, ok := f.Type.AtomicType(); !ok {
				return nil, errors.New("struct " + s.Name + ": field " + f.Name + " has no type")
			}
		}
		x.structs[s.Name] = s
	}
	return x, nil
}

// structDefinitionWrite emits the Forth that defines s in the VM struct table.
func structDefinitionWrite(b *strings.Builder, s *StructDefinition) {
	b.WriteString("StructDefineStart")
	for _, f := range s.Fields {
		t, _ := f.Type.AtomicType()
		b.WriteString(" AtomicType" + forth.AtomicTypeName(t) + " StructDefineFieldAdd")
	}
	b.WriteString(" StructDefineEnd CONSTANT " + StructWordName(s.Name) + "\n")
}

// structFieldOrdinal resolves the field of a field Arg to
// its ordinal and type.
func (c *fnCtx) structFieldOrdinal(a Arg) (int, Type, error) {
	sn, ok := c.structNames[a.variable()]
	if !ok {
		if _, known := c.types[a.variable()]; !known {
			return 0, 0, errors.New("undefined variable " + a.variable())
		}
		return 0, 0, errors.New(a.variable() + " is not a struct, it has no field " + a.structField())
	}
	s := c.index.structDefinition(sn)
	for k, f := range s.Fields {
		if f.Name == a.structField() {
			return k, f.Type, nil
		}
	}
	return 0, 0, errors.New("struct " + sn + " has no field " + a.structField())
}

// structFieldSeek emits the words that put the cursor on field a.
func (c *fnCtx) structFieldSeek(a Arg) {
	k, _, _ := c.structFieldOrdinal(a)
	c.w(strconv.Itoa(c.slots[a.variable()]), " L@ StructCursorOpen ", strconv.Itoa(k), " StructCursorOrdinalSet ")
}

// Translated code holds a CX bool as a Ternary and an i32 as an Int64 or
// Int32, so a field value converts on the way in and a bool on the way out.
func structFieldStoreConvert(t Type) string {
	switch t {
	case I32:
		return "AtomicTypeInt32 AtomicTypeConvert "
	case BOOL:
		return "AtomicTypeBool AtomicTypeConvert "
	}
	return ""
}

func structFieldLoadConvert(t Type) string {
	if t == BOOL {
		return "AtomicTypeTernary AtomicTypeConvert "
	}
	return ""
}
