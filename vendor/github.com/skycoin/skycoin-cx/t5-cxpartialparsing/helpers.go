package partialparsing

import (
	"fmt"
	"io"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-actions"
)

// VIOLATION: state outside State. Owner: t5-cxpartialparsing; moves to ParseMng in phase 2.
var CurrentFile ast.SourceFileGid

func (yylex Lexer) Error(e string) {
	if inREPL {
		fmt.Printf("syntax error: %s\n", e)
		yylex.stop()
		return
	}
	name := ""
	if yylex.tok.yys == IDENTIFIER || yylex.tok.yys == SEMICOLON {
		name = yylex.tok.tok
	}
	text := ast.CompileErrorSyntaxText(e, name)
	if yylex.headerLiteral.lexerHeaderLiteralAfter() {
		unexpected, _, _ := strings.Cut(text, ", expected ")
		text = unexpected + ", composite literal in condition must be parenthesized"
	}
	Program.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeSyntax, CurrentFile, yylex.tok.line, yylex.tok.column, ast.CompileErrorOperands{Program.CXProgramCompileErrorText(text)})
	yylex.stop()
}

func (yylex *Lexer) Stop() {
	yylex.stop()
}

func (yylex *Lexer) Lex(lval *yySymType) int {
	if yylex.tok != nil {
		yylex.tokenBeforeLast = yylex.tokenBefore
		yylex.tokenBefore = lexerToken{lexerTokenKind: yylex.tok.yys, lexerTokenLine: yylex.tok.line, lexerTokenColumn: yylex.tok.column}
	}
	yylex.next()
	yylex.headerLiteral.lexerHeaderLiteralNext(yylex.tok.yys, yylex.tokenBefore.lexerTokenKind, yylex.tok.line)
	lval.scancopy(yylex.tok)
	lineNo = lval.line
	Program.CXProgramCompileErrorTokenSet(CurrentFile, lval.line, lval.column)
	return lval.yys
}

func NewLexer(rdr io.Reader) *Lexer {
	lx := &Lexer{}
	lx.init(rdr, func(l, c int, msg string) {
		// The REPL recovers this panic and keeps going.
		if ast.InREPL {
			panic(fmt.Sprintf("[%d:%d] %s\n", l, c, msg))
		}
		Program.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeMessage, CurrentFile, l, c, ast.CompileErrorOperands{Program.CXProgramCompileErrorText(msg)})
	})
	return lx
}

func (lval *yySymType) scancopy(tok *yySymType) {
	lval.argument = tok.argument
	lval.arguments = tok.arguments
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
	lval.string = tok.string
	lval.stringA = tok.stringA
	lval.tok = tok.tok
	lval.ui16 = tok.ui16
	lval.ui32 = tok.ui32
	lval.ui64 = tok.ui64
	lval.ui8 = tok.ui8
	lval.yys = tok.yys
}

//Unused
//looks like a copy of cxparser/cxgo/lexer.go?
//should definatel be in constants or something
/*
var tokenNames = map[int]string{
	BYTE_LITERAL:    "BYTE_LITERAL",
	BOOLEAN_LITERAL: "BOOLEAN_LITERAL",
	INT_LITERAL:     "INT_LITERAL",
	LONG_LITERAL:    "LONG_LITERAL",
	FLOAT_LITERAL:   "FLOAT_LITERAL",
	DOUBLE_LITERAL:  "DOUBLE_LITERAL",
	FUNC:            "FUNC",
	OP:              "OP",
	LPAREN:          "LPAREN",
	RPAREN:          "RPAREN",
	LBRACE:          "LBRACE",
	RBRACE:          "RBRACE",
	LBRACK:          "LBRACK",
	RBRACK:          "RBRACK",
	IDENTIFIER:      "IDENTIFIER",
	VAR:             "VAR",
	COMMA:           "COMMA",
	PERIOD:          "PERIOD",
	COMMENT:         "COMMENT",
	STRING_LITERAL:  "STRING_LITERAL",
	PACKAGE:         "PACKAGE",
	IF:              "IF",
	ELSE:            "ELSE",
	FOR:             "FOR",
	TYPSTRUCT:       "TYPSTRUCT",
	STRUCT:          "STRUCT",
	SEMICOLON:       "SEMICOLON",
	NEWLINE:         "NEWLINE",
	ASSIGN:          "ASSIGN",
	CASSIGN:         "CASSIGN",
	IMPORT:          "IMPORT",
	RETURN:          "RETURN",
	GOTO:            "GOTO",
	GT_OP:           "GT_OP",
	LT_OP:           "LT_OP",
	GTEQ_OP:         "GTEQ_OP",
	LTEQ_OP:         "LTEQ_OP",
	EQUAL:           "EQUAL",
	COLON:           "COLON",
	NEW:             "NEW",
	EQUALWORD:       "EQUALWORD",
	GTHANWORD:       "GTHANWORD",
	LTHANWORD:       "LTHANWORD",
	GTHANEQ:         "GTHANEQ",
	LTHANEQ:         "LTHANEQ",
	UNEQUAL:         "UNEQUAL",
	AND:             "AND",
	OR:              "OR",
	ADD_OP:          "ADD_OP",
	SUB_OP:          "SUB_OP",
	MUL_OP:          "MUL_OP",
	DIV_OP:          "DIV_OP",
	MOD_OP:          "MOD_OP",
	REF_OP:          "REF_OP",
	NEG_OP:          "NEG_OP",
	AFFVAR:          "AFFVAR",
	PLUSPLUS:        "PLUSPLUS",
	MINUSMINUS:      "MINUSMINUS",
	REMAINDER:       "REMAINDER",
	LEFTSHIFT:       "LEFTSHIFT",
	RIGHTSHIFT:      "RIGHTSHIFT",
	EXP:             "EXP",
	NOT:             "NOT",
	BITXOR_OP:       "BITXOR_OP",
	BITOR_OP:        "BITOR_OP",
	BITCLEAR_OP:     "BITCLEAR_OP",
	PLUSEQ:          "PLUSEQ",
	MINUSEQ:         "MINUSEQ",
	MULTEQ:          "MULTEQ",
	DIVEQ:           "DIVEQ",
	REMAINDEREQ:     "REMAINDEREQ",
	EXPEQ:           "EXPEQ",
	LEFTSHIFTEQ:     "LEFTSHIFTEQ",
	RIGHTSHIFTEQ:    "RIGHTSHIFTEQ",
	BITANDEQ:        "BITANDEQ",
	BITXOREQ:        "BITXOREQ",
	BITOREQ:         "BITOREQ",

	DEC_OP:       "DEC_OP",
	INC_OP:       "INC_OP",
	PTR_OP:       "PTR_OP",
	LEFT_OP:      "LEFT_OP",
	RIGHT_OP:     "RIGHT_OP",
	GE_OP:        "GE_OP",
	LE_OP:        "LE_OP",
	EQ_OP:        "EQ_OP",
	NE_OP:        "NE_OP",
	AND_OP:       "AND_OP",
	OR_OP:        "OR_OP",
	ADD_ASSIGN:   "ADD_ASSIGN",
	AND_ASSIGN:   "AND_ASSIGN",
	LEFT_ASSIGN:  "LEFT_ASSIGN",
	MOD_ASSIGN:   "MOD_ASSIGN",
	MUL_ASSIGN:   "MUL_ASSIGN",
	DIV_ASSIGN:   "DIV_ASSIGN",
	OR_ASSIGN:    "OR_ASSIGN",
	RIGHT_ASSIGN: "RIGHT_ASSIGN",
	SUB_ASSIGN:   "SUB_ASSIGN",
	XOR_ASSIGN:   "XOR_ASSIGN",
	BOOL:         "BOOL",
	F32:          "F32",
	F64:          "F64",
	I8:           "I8",
	I16:          "I16",
	I32:          "I32",
	I64:          "I64",
	STR:          "STR",
	UI8:          "UI8",
	UI16:         "UI16",
	UI32:         "UI32",
	UI64:         "UI64",
	UNION:        "UNION",
	ENUM:         "ENUM",
	CONST:        "CONST",
	CASE:         "CASE",
	DEFAULT:      "DEFAULT",
	SWITCH:       "SWITCH",
	BREAK:        "BREAK",
	CONTINUE:     "CONTINUE",
	TYPE:         "TYPE",

	// Types
	BASICTYPE: "BASICTYPE",
	// Selectors
	SPACKAGE: "SPACKAGE",
	SSTRUCT:  "SSTRUCT",
	SFUNC:    "SFUNC",
	// Removers
	REM:     "REM",
	DEF:     "DEF",
	EXPR:    "EXPR",
	FIELD:   "FIELD",
	CLAUSES: "CLAUSES",
	OBJECT:  "OBJECT",
	OBJECTS: "OBJECTS",
	// Stepping
	STEP:  "STEP",
	PSTEP: "PSTEP",
	TSTEP: "TSTEP",
	// Debugging
	DSTACK:   "DSTACK",
	DPROGRAM: "DPROGRAM",
	DSTATE:   "DSTATE",
	// Affordances
	AFF:   "AFF",
	CAFF:  "CAFF",
	TAG:   "TAG",
	INFER: "INFER",
	VALUE: "VALUE",
	// Pointers
	ADDR: "ADDR",
}
*/

// lexerToken is the kind and position of a token the lexer read.
type lexerToken struct {
	lexerTokenKind   int
	lexerTokenLine   int
	lexerTokenColumn int
}

// varValueRead records the value of var g = value that starts at line and
// column, and returns its index in the values of the lexer. The parser has
// read the token after the value, where the value ends.
func varValueRead(yylex yyLexer, line int, column int) int {
	lexer := yylex.(*Lexer)
	lexer.varValues = append(lexer.varValues, actions.GlobalValue{
		GlobalValueType:        varValueLiteralType(lexer, line, column),
		GlobalValueStartLine:   line,
		GlobalValueStartColumn: column,
		GlobalValueEndLine:     lexer.tok.line,
		GlobalValueEndColumn:   lexer.tok.column,
	})
	return len(lexer.varValues) - 1
}

// varValueLiteralType is the type of the value of var g = value that starts
// at line and column when it is a literal or a negated number literal, and
// types.UNDEFINED otherwise.
func varValueLiteralType(lexer *Lexer, line int, column int) types.Code {
	last := lexer.tokenBefore
	if last.lexerTokenLine == line && last.lexerTokenColumn == column {
		return varLiteralTokenType(last.lexerTokenKind)
	}
	sign := lexer.tokenBeforeLast
	code := varLiteralTokenType(last.lexerTokenKind)
	if sign.lexerTokenLine == line && sign.lexerTokenColumn == column && (sign.lexerTokenKind == SUB_OP || sign.lexerTokenKind == ADD_OP) && code != types.STR && code != types.BOOL {
		return code
	}
	return types.UNDEFINED
}

// varLiteralTokenType is the type of a literal token, as pass two writes it.
func varLiteralTokenType(kind int) types.Code {
	switch kind {
	case STRING_LITERAL:
		return types.STR
	case BOOLEAN_LITERAL:
		return types.BOOL
	case BYTE_LITERAL:
		return types.I8
	case SHORT_LITERAL:
		return types.I16
	case INT_LITERAL:
		return types.I32
	case LONG_LITERAL:
		return types.I64
	case UNSIGNED_BYTE_LITERAL:
		return types.UI8
	case UNSIGNED_SHORT_LITERAL:
		return types.UI16
	case UNSIGNED_INT_LITERAL:
		return types.UI32
	case UNSIGNED_LONG_LITERAL:
		return types.UI64
	case FLOAT_LITERAL:
		return types.F32
	case DOUBLE_LITERAL:
		return types.F64
	}
	return types.UNDEFINED
}

// varValues returns the values var_values holds the indexes of.
func varValues(yylex yyLexer, indexes []int) []actions.GlobalValue {
	lexer := yylex.(*Lexer)
	result := make([]actions.GlobalValue, len(indexes))
	for i, index := range indexes {
		result[i] = lexer.varValues[index]
	}
	return result
}

// varNamesRelease releases the names of a local var spec, which pass one
// does not declare.
func varNamesRelease(names []*ast.CXArgument) {
	for _, name := range names {
		Program.ReleaseCXArgBuilder(name)
	}
}

// lexerHeaderLiteral follows the braces that open after if T or for T, where
// T is an identifier, to tell whether a token comes right after their
// closing brace, as . does in if T{a: 1}.a == 1 {.
type lexerHeaderLiteral struct {
	lexerHeaderLiteralHeader  bool
	lexerHeaderLiteralLine    int
	lexerHeaderLiteralNesting int
	lexerHeaderLiteralBraces  int
	lexerHeaderLiteralClosing bool
	lexerHeaderLiteralClosed  bool
}

// lexerHeaderLiteralNext follows the token kind at line, read after the token
// before. A literal ends on the line of its if or for.
func (header *lexerHeaderLiteral) lexerHeaderLiteralNext(kind int, before int, line int) {
	header.lexerHeaderLiteralClosed = header.lexerHeaderLiteralClosing
	header.lexerHeaderLiteralClosing = false
	switch {
	case kind == IF || kind == FOR:
		*header = lexerHeaderLiteral{lexerHeaderLiteralHeader: true, lexerHeaderLiteralLine: line}
	case header.lexerHeaderLiteralBraces > 0:
		switch kind {
		case LBRACE:
			header.lexerHeaderLiteralBraces++
		case RBRACE:
			header.lexerHeaderLiteralBraces--
			header.lexerHeaderLiteralClosing = header.lexerHeaderLiteralBraces == 0 && line == header.lexerHeaderLiteralLine
		}
	case header.lexerHeaderLiteralHeader:
		switch kind {
		case LPAREN, LBRACK:
			header.lexerHeaderLiteralNesting++
		case RPAREN, RBRACK:
			header.lexerHeaderLiteralNesting--
		case LBRACE:
			if header.lexerHeaderLiteralNesting == 0 {
				header.lexerHeaderLiteralHeader = false
				if before == IDENTIFIER {
					header.lexerHeaderLiteralBraces = 1
				}
			}
		}
	}
}

// lexerHeaderLiteralAfter reports whether the token is right after the
// closing brace of if T {...} or for T {...}.
func (header lexerHeaderLiteral) lexerHeaderLiteralAfter() bool {
	return header.lexerHeaderLiteralClosed
}
