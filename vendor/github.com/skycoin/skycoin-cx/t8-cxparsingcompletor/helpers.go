package parsingcompletor

import (
	"bytes"
	"fmt"
	"io"

	"github.com/skycoin/skycoin-cx/t3-ast"

	"github.com/skycoin/skycoin-cx/t4-actions"
)

func (yylex Lexer) Error(msg string) {
	yylex.stop()
	if ast.InREPL {
		yylex.errorf(msg)
		return
	}
	name := ""
	if yylex.tok.yys == IDENTIFIER || yylex.tok.yys == SEMICOLON {
		name = yylex.tok.tok
	}
	actions.AST.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeSyntax, actions.CurrentFile, yylex.tok.line, yylex.tok.column, ast.CompileErrorOperands{actions.AST.CXProgramCompileErrorText(ast.CompileErrorSyntaxText(msg, name))})
}

func (yylex *Lexer) Lex(lval *yySymType) int {
	yylex.next()
	lval.scancopy(yylex.tok)
	actions.LineNo = lval.line
	actions.LineStr = lval.lineStr
	actions.AST.CXProgramCompileErrorTokenSet(actions.CurrentFile, lval.line, lval.column)
	return lval.yys
}

func (yylex *Lexer) PrintLex(lval *yySymType) (int, int) {
	yylex.next()
	lval.scancopy(yylex.tok)
	actions.LineNo = lval.line
	actions.LineStr = lval.lineStr
	actions.AST.CXProgramCompileErrorTokenSet(actions.CurrentFile, lval.line, lval.column)

	return lval.yys, lval.line
}

func (yylex *Lexer) Next() int {
	yylex.next()
	//fmt.Println(TokenName(yylex.tok.yys))
	return yylex.tok.yys
}

/* func (yylex *Lexer) Stop() {

	yylex.stop() //bug???? # https://github.com/skycoin/cx/issues/529
 }
*/

func NewLexer(rdr io.Reader) *Lexer {
	lx := &Lexer{}
	lx.init(rdr, func(l, c int, msg string) {
		// The REPL recovers this panic and keeps going.
		if ast.InREPL {
			panic(fmt.Sprintf("[%d:%d] %s\n", l, c, msg))
		}
		actions.AST.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeMessage, actions.CurrentFile, l, c, ast.CompileErrorOperands{actions.AST.CXProgramCompileErrorText(msg)})
	})
	return lx
}

func (lval *yySymType) scancopy(tok *yySymType) {
	lval.ReturnExpressions = tok.ReturnExpressions
	lval.SelectStatement = tok.SelectStatement
	lval.SelectStatements = tok.SelectStatements
	lval.argument = tok.argument
	lval.arguments = tok.arguments
	lval.arrayArguments = tok.arrayArguments
	lval.bool = tok.bool
	lval.expression = tok.expression
	lval.expressions = tok.expressions
	lval.f32 = tok.f32
	lval.f64 = tok.f64
	lval.function = tok.function
	lval.i = tok.i
	lval.i16 = tok.i16
	lval.i32 = tok.i32
	lval.i64 = tok.i64
	lval.i8 = tok.i8
	lval.ints = tok.ints
	lval.line = tok.line
	lval.column = tok.column
	lval.lineStr = tok.lineStr
	lval.string = tok.string
	lval.stringA = tok.stringA
	lval.tok = tok.tok
	lval.ui16 = tok.ui16
	lval.ui32 = tok.ui32
	lval.ui64 = tok.ui64
	lval.ui8 = tok.ui8
	lval.yys = tok.yys
}

// Identifiers returns the identifiers of the expression text that can name a
// global or a function. A name after a period is a field or a member of a
// package, and a name before a colon is a key of a struct literal.
func Identifiers(text string) []string {
	lexer := NewLexer(bytes.NewBufferString(text))
	var result []string
	previous := 0
	pending := ""
	for {
		kind := lexer.Next()
		if pending != "" && kind != COLON {
			result = append(result, pending)
		}
		pending = ""
		if kind <= 0 {
			return result
		}
		if kind == IDENTIFIER && previous != PERIOD {
			pending = lexer.tok.tok
		}
		previous = kind
	}
}
