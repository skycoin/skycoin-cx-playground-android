package cxir

// Example is a CX program, its source for reference, and its IR.
type Example struct {
	Name    string
	Source  string
	Program Program
	Output  string
}

// Examples are small CX programs written out as IR by hand.
var Examples = []Example{
	{
		Name: "hello",
		Source: `package main
func main() {
	str.print("hello, world")
}`,
		Program: Program{Functions: []Function{{
			Name:  "main",
			Exprs: []Expr{Print(Str("hello, world"))},
		}}},
		Output: "hello, world\n",
	},
	{
		Name: "fib",
		Source: `package main
func fib(n i32) (r i32) {
	if n < 2 {
		r = n
		return
	}
	r = fib(n-1) + fib(n-2)
}
func main() {
	for i := 0; i < 15; i++ {
		i32.print(fib(i))
	}
}`,
		Program: Program{Functions: []Function{
			{
				Name:    "fib",
				Inputs:  []Arg{Var("n", I32)},
				Outputs: []Arg{Var("r", I32)},
				Locals:  []Arg{Var("c", BOOL), Var("a", I32), Var("b", I32)},
				Exprs: []Expr{
					/* 0 */ Op1(OpLt, Var("c", BOOL), Var("n", I32), Int32(2)),
					/* 1 */ Jmp(Var("c", BOOL), 0, 2),
					/* 2 */ Op1(OpIdentity, Var("r", I32), Var("n", I32)),
					/* 3 */ Return(),
					/* 4 */ Op1(OpSub, Var("a", I32), Var("n", I32), Int32(1)),
					/* 5 */ Call("fib", []Arg{Var("a", I32)}, Var("a", I32)),
					/* 6 */ Op1(OpSub, Var("b", I32), Var("n", I32), Int32(2)),
					/* 7 */ Call("fib", []Arg{Var("b", I32)}, Var("b", I32)),
					/* 8 */ Op1(OpAdd, Var("r", I32), Var("a", I32), Var("b", I32)),
				},
			},
			{
				Name:   "main",
				Locals: []Arg{Var("i", I32), Var("c", BOOL), Var("x", I32)},
				Exprs: []Expr{
					/* 0 */ Op1(OpIdentity, Var("i", I32), Int32(0)),
					/* 1 */ Op1(OpLt, Var("c", BOOL), Var("i", I32), Int32(15)),
					/* 2 */ Jmp(Var("c", BOOL), 0, 4),
					/* 3 */ Call("fib", []Arg{Var("x", I32)}, Var("i", I32)),
					/* 4 */ Print(Var("x", I32)),
					/* 5 */ Op1(OpAdd, Var("i", I32), Var("i", I32), Int32(1)),
					/* 6 */ Goto(-6),
				},
			},
		}},
		Output: "0\n1\n1\n2\n3\n5\n8\n13\n21\n34\n55\n89\n144\n233\n377\n",
	},
	{
		Name: "loop",
		Source: `package main
func main() {
	var sum i32
	for i := 1; i <= 100; i++ {
		sum = sum + i
	}
	i32.print(sum)
	var big i32 = 2147483647
	i32.print(big + 1)
	f64.print(1.0 / 3.0)
	f32.print(100.0 * 9.0 / 5.0 + 32.0)
}`,
		Program: Program{Functions: []Function{{
			Name:   "main",
			Locals: []Arg{Var("sum", I32), Var("i", I32), Var("c", BOOL), Var("big", I32), Var("d", F64), Var("f", F32)},
			Exprs: []Expr{
				/* 0 */ Op1(OpIdentity, Var("sum", I32), Int32(0)),
				/* 1 */ Op1(OpIdentity, Var("i", I32), Int32(1)),
				/* 2 */ Op1(OpLteq, Var("c", BOOL), Var("i", I32), Int32(100)),
				/* 3 */ Jmp(Var("c", BOOL), 0, 3),
				/* 4 */ Op1(OpAdd, Var("sum", I32), Var("sum", I32), Var("i", I32)),
				/* 5 */ Op1(OpAdd, Var("i", I32), Var("i", I32), Int32(1)),
				/* 6 */ Goto(-5),
				/* 7 */ Print(Var("sum", I32)),
				/* 8 */ Op1(OpAdd, Var("big", I32), Int32(2147483647), Int32(1)),
				/* 9 */ Print(Var("big", I32)),
				/* 10 */ Op1(OpDiv, Var("d", F64), Float64(1), Float64(3)),
				/* 11 */ Print(Var("d", F64)),
				/* 12 */ Op1(OpMul, Var("f", F32), Float32(100), Float32(9)),
				/* 13 */ Op1(OpDiv, Var("f", F32), Var("f", F32), Float32(5)),
				/* 14 */ Op1(OpAdd, Var("f", F32), Var("f", F32), Float32(32)),
				/* 15 */ Print(Var("f", F32)),
			},
		}}},
		Output: "5050\n-2147483648\n0.3333333333333333\n212\n",
	},
	{
		Name: "ternary",
		Source: `package main
// Illustrative only, CX has no tri type or switch3. tri is this
// project's extension: true, false or null (unknown).
func report(s tri) {
	switch3 s { true: str.print("secure") false: str.print("open") null: str.print("unknown") }
}
func main() {
	var closed tri = true
	var locked tri = null          // sensor did not answer
	report(closed && locked)       // null
	report(closed && false)        // false, known despite null
	report(locked || true)         // true
	var temp i32 = null
	var hot tri = temp > 30        // null, comparison on unknown
	tri.print(hot)
	tri.print(locked -> closed)    // Kleene implication, true
	locked = true
	report(closed && locked)       // true
}`,
		Program: Program{Functions: []Function{
			{
				Name:   "report",
				Inputs: []Arg{Var("s", TRI)},
				Exprs: []Expr{
					/* 0 */ Jmp3(Var("s", TRI), 0, 2, 4),
					/* 1 */ Print(Str("secure")),
					/* 2 */ Return(),
					/* 3 */ Print(Str("open")),
					/* 4 */ Return(),
					/* 5 */ Print(Str("unknown")),
				},
			},
			{
				Name:   "main",
				Locals: []Arg{Var("closed", TRI), Var("locked", TRI), Var("t", TRI), Var("temp", I32), Var("hot", TRI)},
				Exprs: []Expr{
					/* 0 */ Op1(OpIdentity, Var("closed", TRI), TriLit(true)),
					/* 1 */ Op1(OpIdentity, Var("locked", TRI), NullOf(TRI)),
					/* 2 */ Op1(OpBoolAnd, Var("t", TRI), Var("closed", TRI), Var("locked", TRI)),
					/* 3 */ Call("report", nil, Var("t", TRI)),
					/* 4 */ Op1(OpBoolAnd, Var("t", TRI), Var("closed", TRI), TriLit(false)),
					/* 5 */ Call("report", nil, Var("t", TRI)),
					/* 6 */ Op1(OpBoolOr, Var("t", TRI), Var("locked", TRI), TriLit(true)),
					/* 7 */ Call("report", nil, Var("t", TRI)),
					/* 8 */ Op1(OpIdentity, Var("temp", I32), NullOf(I32)),
					/* 9 */ Op1(OpGt, Var("hot", TRI), Var("temp", I32), Int32(30)),
					/* 10 */ Print(Var("hot", TRI)),
					/* 11 */ Op1(OpImplies, Var("t", TRI), Var("locked", TRI), Var("closed", TRI)),
					/* 12 */ Print(Var("t", TRI)),
					/* 13 */ Op1(OpIdentity, Var("locked", TRI), TriLit(true)),
					/* 14 */ Op1(OpBoolAnd, Var("t", TRI), Var("closed", TRI), Var("locked", TRI)),
					/* 15 */ Call("report", nil, Var("t", TRI)),
				},
			},
		}},
		Output: "unknown\nopen\nsecure\nnull\ntrue\nsecure\n",
	},
}
