package ast

import "strings"

// compileErrorSyntaxTokens names the parser tokens in syntax errors as the
// source spells them. A token that is not listed is named in lower case,
// which is how the keywords are spelled.
var compileErrorSyntaxTokens = map[string]string{
	"$end":                   "end of file",
	"$unk":                   "unknown token",
	"IDENTIFIER":             "name",
	"BOOLEAN_LITERAL":        "literal",
	"BYTE_LITERAL":           "literal",
	"SHORT_LITERAL":          "literal",
	"INT_LITERAL":            "literal",
	"LONG_LITERAL":           "literal",
	"UNSIGNED_BYTE_LITERAL":  "literal",
	"UNSIGNED_SHORT_LITERAL": "literal",
	"UNSIGNED_INT_LITERAL":   "literal",
	"UNSIGNED_LONG_LITERAL":  "literal",
	"FLOAT_LITERAL":          "literal",
	"DOUBLE_LITERAL":         "literal",
	"STRING_LITERAL":         "literal",
	"SEMICOLON":              "semicolon or newline",
	"LPAREN":                 "(",
	"RPAREN":                 ")",
	"LBRACE":                 "{",
	"RBRACE":                 "}",
	"LBRACK":                 "[",
	"RBRACK":                 "]",
	"COMMA":                  "comma",
	"PERIOD":                 ".",
	"COLON":                  ":",
	"ASSIGN":                 "=",
	"CASSIGN":                ":=",
	"INFER":                  "#",
	"EQ_OP":                  "==",
	"NE_OP":                  "!=",
	"NEG_OP":                 "!",
	"GT_OP":                  ">",
	"LT_OP":                  "<",
	"GTEQ_OP":                ">=",
	"LTEQ_OP":                "<=",
	"AND_OP":                 "&&",
	"OR_OP":                  "||",
	"ADD_OP":                 "+",
	"SUB_OP":                 "-",
	"MUL_OP":                 "*",
	"DIV_OP":                 "/",
	"MOD_OP":                 "%",
	"REF_OP":                 "&",
	"BITXOR_OP":              "^",
	"BITOR_OP":               "|",
	"BITCLEAR_OP":            "&^",
	"LEFT_OP":                "<<",
	"RIGHT_OP":               ">>",
	"INC_OP":                 "++",
	"DEC_OP":                 "--",
	"ADD_ASSIGN":             "+=",
	"SUB_ASSIGN":             "-=",
	"MUL_ASSIGN":             "*=",
	"DIV_ASSIGN":             "/=",
	"MOD_ASSIGN":             "%=",
	"AND_ASSIGN":             "&=",
	"OR_ASSIGN":              "|=",
	"XOR_ASSIGN":             "^=",
	"LEFT_ASSIGN":            "<<=",
	"RIGHT_ASSIGN":           ">>=",
}

// compileErrorSyntaxToken returns how a syntax error names the token name.
func compileErrorSyntaxToken(name string) string {
	if text, found := compileErrorSyntaxTokens[name]; found {
		return text
	}
	return strings.ToLower(name)
}

// CompileErrorSyntaxText rewrites a goyacc message, "syntax error:
// unexpected X, expecting A or B", into "unexpected X, expected A or B" with
// the tokens named as the source spells them. text, when not empty, is the
// source text of an unexpected IDENTIFIER or SEMICOLON.
func CompileErrorSyntaxText(message string, text string) string {
	message = strings.TrimPrefix(message, "syntax error")
	message = strings.TrimPrefix(message, ": ")
	unexpected, expecting, _ := strings.Cut(message, ", expecting ")
	token, found := strings.CutPrefix(unexpected, "unexpected ")
	if !found {
		return "unexpected token"
	}
	result := "unexpected " + compileErrorSyntaxToken(token)
	switch {
	case token == "SEMICOLON" && text == "\n":
		result = "unexpected newline"
	case token == "SEMICOLON" && text != "":
		result = "unexpected semicolon"
	case token == "IDENTIFIER" && text != "":
		result += " " + text
	}
	if expecting == "" {
		return result
	}
	result += ", expected "
	for index, token := range strings.Split(expecting, " or ") {
		if index > 0 {
			result += " or "
		}
		result += compileErrorSyntaxToken(token)
	}
	return result
}
