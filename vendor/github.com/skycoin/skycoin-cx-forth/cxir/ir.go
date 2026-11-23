// Package cxir is the smallest useful subset of the CX intermediate
// representation and a translator from it to Forth source.
//
// The structures mirror the CX AST in github.com/skycoin/cx without
// importing it.
//
//	Program  ~ cx/ast/ast_cxprogram.go CXProgram (functions only)
//	Function ~ cx/ast/ast_cxfunction.go CXFunction (Name, Inputs, Outputs, Expressions)
//	Expr     ~ cx/ast/ast_cxatomicoperator.go CXAtomicOperator (Inputs, Outputs, Operator, Label, ThenLines, ElseLines)
//	Arg      ~ cx/ast/ast_cxargument.go CXArgument (name, type, literal value)
//	Type     ~ cx/types/types.go Code (BOOL I32 I64 F32 F64 STR)
//	StructDefinition ~ cx/ast/ast_cxstruct.go CXStruct (Name, Fields)
//	Op       ~ cx/constants/opcodes.go OP_* (jumps, comparisons, arithmetic, bool ops)
//
// Jumps keep CX's relative line offsets. cx/opcodes/op_misc.go opJmp adds
// ThenLines or ElseLines to the current line and the run loop in
// cx/execute/execute.go then steps one line, so the target of a jump on
// line i is i + 1 + offset.
//
// Two extensions carry the ternary logic. TRI is a type whose values are
// true, false and null, and an Arg literal may be Null for any type. A JMP
// whose condition is null goes to NullLines when HasNull is set, otherwise
// it takes the ElseLines path.
//
// A struct variable, of type TypeStruct, holds a StructRef to an instance
// in the VM struct space, and an Arg named p.x is field x of p. Each
// StructDefinition becomes a VM struct type. A function makes an instance
// for each struct local and output, and frees them on return when it
// returns no struct. Struct arguments pass the ref, so a callee shares its
// caller's instance. Fields are atomic, nested structs are not supported.
//
// OpHost calls a word the host program defines, such as net.request. It
// is how CX code reaches anything outside the VM, the way CX itself calls
// its native functions.
package cxir

// Type is a CX value type.
type Type uint8

// Types, a subset of cx/types Code plus TRI.
const (
	BOOL Type = iota + 1
	I32
	I64
	F32
	F64
	STR
	TRI
	TypeStruct // a CX struct instance, the variable's Arg.S names the struct
)

// Op is an operator. The names follow cx/constants/opcodes.go.
type Op uint16

// Operators.
const (
	OpIdentity Op = iota + 1 // OP_IDENTITY, out = in
	OpJmp                    // OP_JMP, conditional relative jump
	OpGoto                   // OP_GOTO, unconditional relative jump by ThenLines
	OpEqual                  // OP_EQUAL
	OpUnequal                // OP_UNEQUAL
	OpLt                     // OP_LT
	OpGt                     // OP_GT
	OpLteq                   // OP_LTEQ
	OpGteq                   // OP_GTEQ
	OpMul                    // OP_MUL
	OpDiv                    // OP_DIV
	OpMod                    // OP_MOD
	OpAdd                    // OP_ADD
	OpSub                    // OP_SUB
	OpNeg                    // OP_NEG
	OpBoolOr                 // OP_BOOL_OR, Kleene on TRI
	OpBoolAnd                // OP_BOOL_AND, Kleene on TRI
	OpBoolNot                // OP_BOOL_NOT, Kleene on TRI
	OpImplies                // extension, Kleene implication
	OpPrint                  // CX's typed print builtins such as i32.print, registered in cx/opcodes/opcodes_init.go
	OpCall                   // call of a user function, Expr.Call names it
	OpReturn                 // leave the function, outputs hold the results
	OpHost                   // extension, call of a word the host defines, Expr.Call names it
)

// Arg is a variable reference or a literal.
type Arg struct {
	Name string
	Type Type
	Lit  bool
	Null bool // literal null, extension
	I    int64
	F    float64
	S    string // a str literal, or the struct of a TypeStruct variable
	B    bool
}

// Var references a variable.
func Var(name string, t Type) Arg { return Arg{Name: name, Type: t} }

// Int32 is an i32 literal.
func Int32(v int32) Arg { return Arg{Type: I32, Lit: true, I: int64(v)} }

// Int64 is an i64 literal.
func Int64(v int64) Arg { return Arg{Type: I64, Lit: true, I: v} }

// Float32 is an f32 literal.
func Float32(v float32) Arg { return Arg{Type: F32, Lit: true, F: float64(v)} }

// Float64 is an f64 literal.
func Float64(v float64) Arg { return Arg{Type: F64, Lit: true, F: v} }

// Str is a str literal.
func Str(s string) Arg { return Arg{Type: STR, Lit: true, S: s} }

// Bool is a bool literal.
func Bool(b bool) Arg { return Arg{Type: BOOL, Lit: true, B: b} }

// NullOf is a null literal of type t.
func NullOf(t Type) Arg { return Arg{Type: t, Lit: true, Null: true} }

// Expr is one IR line.
type Expr struct {
	Op        Op
	Inputs    []Arg
	Outputs   []Arg
	Call      string
	ThenLines int
	ElseLines int
	NullLines int
	HasNull   bool
}

// Function is a CX function. Inputs, Outputs and Locals together are its
// variables.
type Function struct {
	Name    string
	Inputs  []Arg
	Outputs []Arg
	Locals  []Arg
	Exprs   []Expr
}

// Program is a list of functions and the structs they use. main is the
// entry point.
type Program struct {
	Functions []Function
	Structs   []StructDefinition
}

// Op1 builds out = op(in...).
func Op1(op Op, out Arg, in ...Arg) Expr { return Expr{Op: op, Inputs: in, Outputs: []Arg{out}} }

// Jmp builds a conditional jump.
func Jmp(cond Arg, then, els int) Expr {
	return Expr{Op: OpJmp, Inputs: []Arg{cond}, ThenLines: then, ElseLines: els}
}

// Jmp3 builds a three way jump on a ternary condition.
func Jmp3(cond Arg, then, els, null int) Expr {
	return Expr{Op: OpJmp, Inputs: []Arg{cond}, ThenLines: then, ElseLines: els, NullLines: null, HasNull: true}
}

// Goto builds an unconditional jump.
func Goto(lines int) Expr { return Expr{Op: OpGoto, ThenLines: lines} }

// Print builds a print of one value followed by a newline.
func Print(a Arg) Expr { return Expr{Op: OpPrint, Inputs: []Arg{a}} }

// Call builds outs = fn(ins).
func Call(fn string, outs []Arg, ins ...Arg) Expr {
	return Expr{Op: OpCall, Call: fn, Inputs: ins, Outputs: outs}
}

// Host builds outs = word(ins) for a word the host defines with forth.VM.Define.
func Host(word string, outs []Arg, ins ...Arg) Expr {
	return Expr{Op: OpHost, Call: word, Inputs: ins, Outputs: outs}
}

// Return builds a return.
func Return() Expr { return Expr{Op: OpReturn} }

// TriLit is a ternary true or false literal. Use NullOf(TRI) for null.
func TriLit(b bool) Arg { return Arg{Type: TRI, Lit: true, B: b} }
