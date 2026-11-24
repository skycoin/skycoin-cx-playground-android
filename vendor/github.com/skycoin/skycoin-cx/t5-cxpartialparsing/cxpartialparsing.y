%{
	package partialparsing
	import (
		"bytes"
	    "github.com/skycoin/skycoin-cx/t3-ast"
	    "github.com/skycoin/skycoin-cx/t1-constants"
	    "github.com/skycoin/skycoin-cx/t0-types"
	    stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
		"github.com/skycoin/skycoin-cx/t4-actions"
	)

        /*
        This is a computer generated file
        - partialparsing.go is generated from partialparsing.y via 
        - go tool goyacc -o t5-cxpartialparsing/cxpartialparsing.go t5-cxpartialparsing/cxpartialparsing.y
        */

	// OBJMODEL: *CxIrState passed as a parameter, owned by main, phase 5 #372.
	// Now a pointer at rest, site S1392.
	var Program *ast.CXProgram

	// VIOLATION: state outside State. Owner: t5-cxpartialparsing; moves to ParseMng in phase 2.
	var lineNo int = -1
	var replMode bool = false
	var inREPL bool = false
	var inFn bool = false
	var fileName string

	// Parse() is the function that is called from main().
	// It is needed because yyParse is not exported.
	func Parse(code string) int {
		yyErrorVerbose = true
		codeBuf := bytes.NewBufferString(code)
		return yyParse(NewLexer(codeBuf))
	}

	func PreFunctionDeclaration (fnIdx ast.CXFunctionIndex, inputs []*ast.CXArgument, outputs []*ast.CXArgument) {
                // OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
                // Now an arena getter, site S1393.
                fn:=actions.AST.GetFunctionFromArray(fnIdx)
		// adding inputs, outputs
		for _, inp := range inputs {
			fn.AddInput(actions.AST,inp)
		}
		for _, out := range outputs {
			fn.AddOutput(actions.AST,out)
		}
	}
	
%}

%union {
	i int
	i8 int8
	i16 int16
	i32 int32
	i64 int64
	ui8 uint8
	ui16 uint16
	ui32 uint32
	ui64 uint64
	f32 float32
	f64 float64
	tok string
	bool bool
	string string
	stringA []string
	ints    []int

	line int
	column int

	// OBJMODEL: CXArgumentGid, the parser value stack lives across Allocs, phase 4 #326.
	// Now a pointer at rest, site S1394.
	argument *ast.CXArgument
	// OBJMODEL: CXArgumentGid, the parser value stack lives across Allocs, phase 4 #326.
	// Now a pointer at rest, site S1395.
	arguments []*ast.CXArgument

	// OBJMODEL: CXExpressionGid, the parser value stack lives across Allocs, phase 4 #346.
	// Now a pointer at rest, site S1396.
	expression *ast.CXExpression
	// OBJMODEL: CXExpressionGid, the parser value stack lives across Allocs, phase 4 #346.
	// Now a pointer at rest, site S1397.
	expressions []*ast.CXExpression

        // OBJMODEL: CXFunctionGid, the parser value stack lives across Allocs, phase 4 #351.
        // Now a pointer at rest, site S1398.
        function *ast.CXFunction
        functionIndex ast.CXFunctionIndex
}

%token  <bool>          BOOLEAN_LITERAL
%token  <i8>            BYTE_LITERAL
%token  <i16>           SHORT_LITERAL
%token  <i32>           INT_LITERAL
%token  <i64>           LONG_LITERAL
%token  <ui8>           UNSIGNED_BYTE_LITERAL
%token  <ui16>          UNSIGNED_SHORT_LITERAL
%token  <ui32>          UNSIGNED_INT_LITERAL
%token  <ui64>          UNSIGNED_LONG_LITERAL
%token  <f32>           FLOAT_LITERAL
%token  <f64>           DOUBLE_LITERAL
%token  <tok>           FUNC OP LPAREN RPAREN LBRACE RBRACE LBRACK RBRACK IDENTIFIER
                        VAR GLOBAL COMMA PERIOD COMMENT STRING_LITERAL PACKAGE IF ELSE FOR TYPSTRUCT STRUCT
                        SEMICOLON NEWLINE
                        ASSIGN CASSIGN IMPORT RETURN GOTO GT_OP LT_OP GTEQ_OP LTEQ_OP EQUAL COLON NEW
                        EQUALWORD GTHANWORD LTHANWORD
                        GTHANEQ LTHANEQ UNEQUAL AND OR
                        ADD_OP SUB_OP MUL_OP DIV_OP MOD_OP REF_OP NEG_OP AFFVAR
                        PLUSPLUS MINUSMINUS REMAINDER LEFTSHIFT RIGHTSHIFT EXP
                        NOT
                        BITXOR_OP BITOR_OP BITCLEAR_OP
                        PLUSEQ MINUSEQ MULTEQ DIVEQ REMAINDEREQ EXPEQ
                        LEFTSHIFTEQ RIGHTSHIFTEQ BITANDEQ BITXOREQ BITOREQ

                        DEC_OP INC_OP PTR_OP LEFT_OP RIGHT_OP
                        GE_OP LE_OP EQ_OP NE_OP AND_OP OR_OP
                        ADD_ASSIGN AND_ASSIGN LEFT_ASSIGN MOD_ASSIGN
                        MUL_ASSIGN DIV_ASSIGN OR_ASSIGN RIGHT_ASSIGN
                        SUB_ASSIGN XOR_ASSIGN
                        BOOL F32 F64
                        I8 I16 I32 I64
                        STR
                        UI8 UI16 UI32 UI64
                        UNION ENUM CONST CASE DEFAULT SWITCH BREAK CONTINUE
                        TYPE
                        
                        /* Types */
                        BASICTYPE

                        /* Removers */
                        REM DEF EXPR FIELD CLAUSES OBJECT OBJECTS

                        /* Debugging */
                        DSTACK DPROGRAM DSTATE
                        /* Affordances */
                        AFF CAFF TAG INFER VALUE
                        /* Pointers */
                        ADDR

                        yyDefault

%type   <tok>           unary_operator
%type   <i>             type_specifier
%type   <argument>      declaration_specifiers
%type   <argument>      element_specifiers
%type   <argument>      array_element
%type   <argument>      declarator
%type   <argument>      direct_declarator
%type   <argument>      parameter_declaration
%type   <arguments>     parameter_type_list
%type   <arguments>     function_parameters
%type   <arguments>     function_results
%type   <arguments>     result_type_list
%type   <arguments>     parameter_list
%type   <arguments>     fields
%type   <arguments>     struct_fields
%type   <arguments>     id_list
%type   <arguments>     types_list
%type   <arguments>     var_names
%type   <ints>          var_values
                                                
%type   <functionIndex>      function_header

%type   <ints>          indexing_literal
%type   <ints>          indexing_slice_literal


//%start
                        
%%

translation_unit:
                external_declaration
        |       translation_unit external_declaration
        ;

external_declaration:
                package_declaration
        |       global_declaration
        |       function_declaration
        |       import_declaration
        |       struct_declaration
        ;

// A var declaration declares one or more names, or a parenthesized group of
// such specs, as in Go (https://go.dev/ref/spec#Variable_declarations).
global_declaration:
                VAR global_var_spec SEMICOLON
        |       VAR LPAREN global_var_specs RPAREN SEMICOLON
        |       VAR LPAREN RPAREN SEMICOLON
                ;

global_var_specs:
                global_var_spec
        |       global_var_specs SEMICOLON global_var_spec
        |       global_var_specs SEMICOLON
                ;

global_var_spec:
                var_names declaration_specifiers
                {
			actions.DeclareGlobals(Program, $1, $2, nil, nil, false)
                }
        |       var_names declaration_specifiers ASSIGN var_values
                {
			actions.DeclareGlobals(Program, $1, $2, nil, varValues(yylex, $4), true)
                }
        |       var_names ASSIGN var_values
                {
			actions.DeclareGlobals(Program, $1, nil, nil, varValues(yylex, $3), true)
                }
                ;

var_names:
                declarator
                {
			$$ = []*ast.CXArgument{$1}
                }
        |       var_names COMMA declarator
                {
			$$ = append($1, $3)
                }
                ;

// var_values holds the index of each value in the values the lexer read, see
// varValueRead.
var_values:
                conditional_expression
                {
			$$ = []int{varValueRead(yylex, $<line>1, $<column>1)}
                }
        |       var_values COMMA conditional_expression
                {
			$$ = append($1, varValueRead(yylex, $<line>3, $<column>3))
                }
                ;

struct_declaration:
                TYPE IDENTIFIER STRUCT struct_fields
                {
			actions.DeclareStruct(Program,$2, $4, CurrentFile, $<line>2, $<column>2)
                }
                ;

// A semicolon may be omitted before the closing brace, as in Go.
struct_fields:
                LBRACE RBRACE SEMICOLON
                { $$ = nil }
        |       LBRACE fields RBRACE SEMICOLON
                { $$ = $2 }
        |       LBRACE fields SEMICOLON RBRACE SEMICOLON
                { $$ = $2 }
        ;

fields:         parameter_declaration
                {
			// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
			// Now a slice of objects, site S1399.
			$$ = []*ast.CXArgument{$1}
                }
        |       fields SEMICOLON parameter_declaration
                {
			$$ = append($1, $3)
                }
        ;

package_declaration:
                PACKAGE IDENTIFIER SEMICOLON
                {
			actions.DeclarePackage(Program,$2)
                }
                ;

import_declaration:
                IMPORT STRING_LITERAL SEMICOLON
                {
			actions.DeclareImportAt(Program,$2, CurrentFile, lineNo, $<column>2)
                }
        // The imports into scope of #487: import . "p", import file . "p",
        // import (a, b) "p" and import file (a, b) "p".
        |       IMPORT PERIOD STRING_LITERAL SEMICOLON
                {
			actions.DeclareImportScoped(Program, $3, "", 0, 0, nil, CurrentFile, $<line>3, $<column>3)
                }
        |       IMPORT IDENTIFIER PERIOD STRING_LITERAL SEMICOLON
                {
			actions.DeclareImportScoped(Program, $4, $2, $<line>2, $<column>2, nil, CurrentFile, $<line>4, $<column>4)
                }
        |       IMPORT LPAREN import_names RPAREN STRING_LITERAL SEMICOLON
                {
			actions.DeclareImportScoped(Program, $5, "", 0, 0, actions.ImportScopedNamesTake(), CurrentFile, $<line>5, $<column>5)
                }
        |       IMPORT IDENTIFIER LPAREN import_names RPAREN STRING_LITERAL SEMICOLON
                {
			actions.DeclareImportScoped(Program, $6, $2, $<line>2, $<column>2, actions.ImportScopedNamesTake(), CurrentFile, $<line>6, $<column>6)
                }
                ;

import_names:
                IDENTIFIER
                {
			actions.ImportScopedNamesTake()
			actions.ImportScopedNamesAdd($1, $<line>1, $<column>1)
                }
        |       import_names COMMA IDENTIFIER
                {
			actions.ImportScopedNamesAdd($3, $<line>3, $<column>3)
                }
                ;

function_header:
                FUNC IDENTIFIER
                {
			if pkg, err := Program.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now a constructor call, site S1400.
				fn := ast.MakeFunction(Program, Program.CXProgramNameIntern($2), CurrentFile, $<line>2)
				fn.FileColumn = $<column>2
				_,fnIdx:=pkg.AddFunction(Program,fn)
                                pkg.CurrentFunction=fnIdx

                                $$ = fnIdx
			} else {
				panic(err)
			}
                }
        |       FUNC LPAREN parameter_type_list RPAREN IDENTIFIER
                {
			receiver := actions.MethodReceiver(Program, $3)

			receiverName := actions.MethodReceiverName(Program, receiver[0], CurrentFile, $<line>5, $<column>5)
			methodName := Program.CXProgramNameIntern($5)

			if pkg, err := Program.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now a constructor call, site S1401.
				fn := ast.MakeFunction(Program, Program.CXProgramMethodNameIntern(receiverName, methodName), CurrentFile, $<line>5)
				fn.FileColumn = $<column>5
				_,fnIdx:=pkg.AddMethod(Program,fn,receiverName,methodName)
                                // OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
                                // Now an arena getter, site S1402.
                                newFn:=Program.GetFunctionFromArray(fnIdx)
                                newFn.AddInput(Program,receiver[0])
                                
                                pkg.CurrentFunction=fnIdx

                                $$ = fnIdx
			} else {
				panic(err)
			}
                }
        ;

function_parameters:
                LPAREN RPAREN
                { $$ = nil }
        |       LPAREN parameter_type_list RPAREN
                { $$ = $2 }
                ;

// A result list holds names and types, (r i32, s str), or only types,
// (i32, str), as in Go. Unnamed results get hidden names.
function_results:
                function_parameters
        |       LPAREN result_type_list RPAREN
                {
			$$ = actions.FunctionResultsUnnamed(Program, $2)
                }
        |       LPAREN result_type_list COMMA RPAREN
                {
			$$ = actions.FunctionResultsUnnamed(Program, $2)
                }
        ;

result_type_list:
                declaration_specifiers
                {
			$$ = []*ast.CXArgument{$1}
                }
        |       result_type_list COMMA declaration_specifiers
                {
			$$ = append($1, $3)
                }
        ;

function_declaration:
                function_header function_parameters compound_statement SEMICOLON
                {
			PreFunctionDeclaration($1, $2, nil)
                }
        |       function_header function_parameters function_results compound_statement SEMICOLON
                {
			PreFunctionDeclaration($1, $2, $3)
                }
        |       function_header function_parameters declaration_specifiers compound_statement SEMICOLON
                {
			PreFunctionDeclaration($1, $2, actions.FunctionResultUnnamed(Program, $3))
                }
        ;

parameter_type_list:
                //parameter_list COMMA ELLIPSIS
		parameter_list
	|	parameter_list COMMA
                ;

parameter_list:
                parameter_declaration
                {
			// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
			// Now a slice of objects, site S1403.
			$$ = []*ast.CXArgument{$1}
                }
	|       parameter_list COMMA parameter_declaration
                {
			$$ = append($1, $3)
                }
                ;

parameter_declaration:
                declarator declaration_specifiers
                {
			details := Program.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet($2.ArgDetails)
			// The parameter is at its name.
			details.FileName, details.FileLine, details.FileColumn = CurrentFile, $<line>1, $<column>1
			$2.Name = $1.Name
			$2.Package = $1.Package
			Program.ReleaseCXArgBuilder($1)
                        
			$$ = $2
                }
                ;

declarator:     direct_declarator
                ;

direct_declarator:
                IDENTIFIER
                {
			if pkg, err := Program.GetCurrentPackage(); err == nil {
				_, arg := ast.MakeArgument(Program, stringpool.StringGidNil, CurrentFile, lineNo)
				arg.SetType(types.UNDEFINED)
				arg.Name = Program.CXProgramNameIntern($1)
				arg.Package = ast.CXPackageIndex(pkg.Index)
				$$ = arg
			} else {
				panic(err)
			}
                }
	|       LPAREN declarator RPAREN
                { $$ = $2 }
                ;

id_list:	IDENTIFIER
		{
			arg := actions.DeclarationSpecifiersStruct(Program,$1, "", false, CurrentFile, $<line>1, $<column>1)
			// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
			// Now a slice of objects, site S1405.
			$$ = []*ast.CXArgument{arg}
		}
	|	type_specifier
		{
			arg := actions.DeclarationSpecifiersBasic(Program, types.Code($1))			 
			// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
			// Now a slice of objects, site S1406.
			$$ = []*ast.CXArgument{arg}
		}
	|	id_list COMMA IDENTIFIER
		{
			arg := actions.DeclarationSpecifiersStruct(Program,$3, "", false, CurrentFile, $<line>3, $<column>3)
			$$ = append($1, arg)
		}
	|	id_list COMMA type_specifier
		{
			arg := actions.DeclarationSpecifiersBasic(Program, types.Code($3))
			$$ = append($1, arg)
		}
	;

types_list:
		LPAREN id_list RPAREN
		{
			$$ = $2
		}
	|	LPAREN RPAREN
		{
			$$ = nil
		}
	;

declaration_specifiers:
                element_specifiers
        |       indexing_literal array_element
                {
			$$ = actions.DeclarationSpecifiers($2, types.Cast_sint_to_sptr($1), constants.DECL_ARRAY)
                }
                ;

element_specifiers:
		FUNC types_list types_list
		{
			_, arg := ast.MakeArgument(Program, stringpool.StringGidNil, actions.CurrentFile, actions.LineNo)
			arg.SetType(types.FUNC)
			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now an arena insert, site S1408.
			arg.Inputs = Program.AddPointerArgsToCXArgsArray($2)
			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now an arena insert, site S1409.
			arg.Outputs = Program.AddPointerArgsToCXArgsArray($3)
			$$ = actions.DeclarationSpecifiers(arg, []types.Pointer{0}, constants.DECL_FUNC)
		}
        |       MUL_OP declaration_specifiers
                {
			$$ = actions.DeclarationSpecifiers($2, []types.Pointer{0}, constants.DECL_POINTER)
                }
        |       LBRACK RBRACK declaration_specifiers
                {
			$$ = actions.DeclarationSpecifiers($3, []types.Pointer{0}, constants.DECL_SLICE)
                }
        |       type_specifier
                {
			$$ = actions.DeclarationSpecifiersBasic(Program, types.Code($1))
                }
        |       IDENTIFIER
                {
			$$ = actions.DeclarationSpecifiersStruct(Program,$1, "", false, CurrentFile, $<line>1, $<column>1)
                }
        |       IDENTIFIER PERIOD IDENTIFIER
                {
			$$ = actions.DeclarationSpecifiersStruct(Program,$3, $1, true, CurrentFile, $<line>1, $<column>1)
                }
	|       type_specifier PERIOD IDENTIFIER
		{
			$$ = actions.DeclarationSpecifiersStruct(Program,$3, types.Code($1).Name(), true, CurrentFile, $<line>1, $<column>1)
		}
                ;

// array_element is what an array may hold: a basic or struct type, a struct
// of another package, or a pointer to one of them.
array_element:
                type_specifier
                {
			$$ = actions.DeclarationSpecifiersBasic(Program, types.Code($1))
                }
        |       IDENTIFIER
                {
			$$ = actions.DeclarationSpecifiersStruct(Program,$1, "", false, CurrentFile, $<line>1, $<column>1)
                }
        |       IDENTIFIER PERIOD IDENTIFIER
                {
			$$ = actions.DeclarationSpecifiersStruct(Program,$3, $1, true, CurrentFile, $<line>1, $<column>1)
                }
        |       type_specifier PERIOD IDENTIFIER
                {
			$$ = actions.DeclarationSpecifiersStruct(Program,$3, types.Code($1).Name(), true, CurrentFile, $<line>1, $<column>1)
                }
        |       MUL_OP declaration_specifiers
                {
			$$ = actions.DeclarationSpecifiers($2, []types.Pointer{0}, constants.DECL_POINTER)
                }
        |       LBRACK RBRACK declaration_specifiers
                {
			$$ = actions.DeclarationSpecifiers($3, []types.Pointer{0}, constants.DECL_SLICE)
                }
                ;

type_specifier:
                AFF
                { $$ = int(types.AFF) }
        |       BOOL
                { $$ = int(types.BOOL) }
        |       STR
                { $$ = int(types.STR) }
        |       F32
                { $$ = int(types.F32) }
        |       F64
                { $$ = int(types.F64) }
        |       I8
                { $$ = int(types.I8) }
        |       I16
                { $$ = int(types.I16) }
        |       I32
                { $$ = int(types.I32) }
        |       I64
                { $$ = int(types.I64) }
        |       UI8
                { $$ = int(types.UI8) }
        |       UI16
                { $$ = int(types.UI16) }
        |       UI32
                { $$ = int(types.UI32) }
        |       UI64
                { $$ = int(types.UI64) }
	/* |       struct_or_union_specifier */
        /*         { */
        /*             $$ = "struct" */
        /*         } */
	/* |       enum_specifier */
        /*         { */
        /*             $$ = "enum" */
        /*         } */
	/* |       TYPEDEF_NAME // check */
                ;



struct_literal_fields:
                // empty
        |       IDENTIFIER COLON conditional_expression
        |       struct_literal_fields COMMA IDENTIFIER COLON conditional_expression
                ;

literal_element_list:
		literal_element
	|       literal_element_list COMMA literal_element
                ;

// A list of elements or fields may end in a comma, which Go requires when the
// closing brace is on the next line (https://go.dev/ref/spec#Semicolons).
list_end:
                RBRACE
        |       COMMA RBRACE
                ;

// An elided element, {1, 2} or {x: 1}, takes the element type of its literal.
literal_element:
		assignment_expression
        |       LBRACE literal_element_list list_end
        |       LBRACE struct_literal_fields list_end
                ;

indexing_literal:
		LBRACK INT_LITERAL RBRACK
		{
			$$ = []int{int($2)}
		}
        |       indexing_literal LBRACK INT_LITERAL RBRACK
		{
			$$ = append($1, int($3))
		}
		;

indexing_slice_literal:
		LBRACK RBRACK
		{
			$$ = []int{0}
		}
        |       indexing_slice_literal LBRACK RBRACK
		{
			$$ = append($1, 0)
		}
		;

// expressions
array_literal_expression:
                indexing_literal IDENTIFIER LBRACE literal_element_list list_end
        |       indexing_literal IDENTIFIER LBRACE RBRACE
        |       indexing_literal type_specifier LBRACE literal_element_list list_end
        |       indexing_literal type_specifier LBRACE RBRACE
        ;

slice_literal_expression:
                indexing_slice_literal IDENTIFIER LBRACE literal_element_list list_end
        |       indexing_slice_literal IDENTIFIER LBRACE RBRACE
        |       indexing_slice_literal type_specifier LBRACE literal_element_list list_end
        |       indexing_slice_literal type_specifier LBRACE RBRACE
                ;

// A literal of a type with a pointer or with both array and slice
// dimensions, such as []*T or [2][]i32.
compound_literal_type:
                indexing_literal MUL_OP declaration_specifiers
        |       indexing_literal LBRACK RBRACK declaration_specifiers
        |       indexing_slice_literal MUL_OP declaration_specifiers
        |       indexing_slice_literal indexing_literal array_element
                ;

compound_literal_expression:
                compound_literal_type LBRACE literal_element_list list_end
        |       compound_literal_type LBRACE RBRACE
                ;



/* infer_action_arg: */
/*                 MUL_OP GT_OP assignment_expression */
/*         |       MUL_OP GT_OP MUL_OP */
/*         ; */

infer_action_arg:
                IDENTIFIER
        |       INT_LITERAL
	|	type_specifier PERIOD IDENTIFIER
        ;

infer_action:
                IDENTIFIER LPAREN infer_action_arg COMMA IDENTIFIER RPAREN
	|	IDENTIFIER LPAREN infer_action_arg RPAREN
	|	IDENTIFIER LPAREN infer_action RPAREN
	|	IDENTIFIER LPAREN infer_action COMMA infer_action RPAREN
        ;

infer_actions:
                infer_action SEMICOLON
        |       infer_actions infer_action SEMICOLON
                ;

/* infer_target: */
/*                 IDENTIFIER LPAREN IDENTIFIER RPAREN SEMICOLON */
/*         ; */

/* infer_targets: */
/*                 infer_target */
/*         |       infer_targets infer_target */
/*         ; */

infer_clauses:
        |       infer_actions
        /* |       infer_targets */
                ;

primary_expression:
                IDENTIFIER
        /* |       IDENTIFIER LBRACE struct_literal_fields RBRACE */
        |       INFER LBRACE infer_clauses RBRACE
        |       STRING_LITERAL
        |       BOOLEAN_LITERAL
        |       BYTE_LITERAL
        |       SHORT_LITERAL
        |       INT_LITERAL
        |       LONG_LITERAL
        |       UNSIGNED_BYTE_LITERAL
        |       UNSIGNED_SHORT_LITERAL
        |       UNSIGNED_INT_LITERAL
        |       UNSIGNED_LONG_LITERAL
        |       FLOAT_LITERAL
        |       DOUBLE_LITERAL
        |       LPAREN expression RPAREN
        |       array_literal_expression
        |       slice_literal_expression
        |       compound_literal_expression
                ;

after_period:   type_specifier
        |       IDENTIFIER
        ;

// A struct literal is an operand, as in Go. An if or for header uses the _nc
// rules, which hold no bare struct literal (skycoin-cx#21).
postfix_expression:
                primary_expression
        |       struct_literal_value
	|       postfix_expression LBRACK assignment_expression RBRACK
        |       type_specifier PERIOD after_period
	|       postfix_expression LPAREN RPAREN
	|       postfix_expression LPAREN argument_expression_list RPAREN
	|       postfix_expression LPAREN argument_expression_list COMMA RPAREN
	|       postfix_expression INC_OP
        |       postfix_expression DEC_OP
        |       postfix_expression PERIOD IDENTIFIER
                ;

struct_literal_value:
		IDENTIFIER LBRACE struct_literal_fields list_end
        |       postfix_expression PERIOD IDENTIFIER LBRACE struct_literal_fields list_end
        ;

argument_expression_list:
                assignment_expression
	|       argument_expression_list COMMA assignment_expression
                ;

unary_expression:
                postfix_expression
	|       INC_OP unary_expression
	|       DEC_OP unary_expression
	|       unary_operator unary_expression
                ;

unary_operator:
                REF_OP
	|       MUL_OP
	|       ADD_OP
	|       SUB_OP
	|       NEG_OP
                ;

// Binary operator precedence follows https://go.dev/ref/spec#Operator_precedence
multiplicative_expression:
                unary_expression
	|       multiplicative_expression MUL_OP unary_expression
	|       multiplicative_expression DIV_OP unary_expression
	|       multiplicative_expression MOD_OP unary_expression
	|       multiplicative_expression LEFT_OP unary_expression
	|       multiplicative_expression RIGHT_OP unary_expression
	|       multiplicative_expression REF_OP unary_expression
	|       multiplicative_expression BITCLEAR_OP unary_expression
                ;

additive_expression:
                multiplicative_expression
	|       additive_expression ADD_OP multiplicative_expression
	|       additive_expression SUB_OP multiplicative_expression
	|       additive_expression BITOR_OP multiplicative_expression
	|       additive_expression BITXOR_OP multiplicative_expression
                ;

relational_expression:
                additive_expression
	|       relational_expression EQ_OP additive_expression
	|       relational_expression NE_OP additive_expression
	|       relational_expression LT_OP additive_expression
	|       relational_expression GT_OP additive_expression
	|       relational_expression LTEQ_OP additive_expression
	|       relational_expression GTEQ_OP additive_expression
                ;

logical_and_expression:
                relational_expression
	|       logical_and_expression AND_OP relational_expression
                ;

logical_or_expression:
                logical_and_expression
	|       logical_or_expression OR_OP logical_and_expression
                ;

conditional_expression:
                logical_or_expression
	|       logical_or_expression '?' expression COLON conditional_expression
                ;

// The expression rules of an if or for header, which hold no bare struct
// literal. One in parentheses, brackets or braces is a full expression.
postfix_nc:
                primary_expression
	|       postfix_nc LBRACK assignment_expression RBRACK
        |       type_specifier PERIOD after_period
	|       postfix_nc LPAREN RPAREN
	|       postfix_nc LPAREN argument_expression_list RPAREN
	|       postfix_nc LPAREN argument_expression_list COMMA RPAREN
	|       postfix_nc INC_OP
        |       postfix_nc DEC_OP
        |       postfix_nc PERIOD IDENTIFIER
                ;

unary_nc:
                postfix_nc
	|       INC_OP unary_nc
	|       DEC_OP unary_nc
	|       unary_operator unary_nc
                ;

// Binary operator precedence follows https://go.dev/ref/spec#Operator_precedence
multiplicative_nc:
                unary_nc
	|       multiplicative_nc MUL_OP unary_nc
	|       multiplicative_nc DIV_OP unary_nc
	|       multiplicative_nc MOD_OP unary_nc
	|       multiplicative_nc LEFT_OP unary_nc
	|       multiplicative_nc RIGHT_OP unary_nc
	|       multiplicative_nc REF_OP unary_nc
	|       multiplicative_nc BITCLEAR_OP unary_nc
                ;

additive_nc:
                multiplicative_nc
	|       additive_nc ADD_OP multiplicative_nc
	|       additive_nc SUB_OP multiplicative_nc
	|       additive_nc BITOR_OP multiplicative_nc
	|       additive_nc BITXOR_OP multiplicative_nc
                ;

relational_nc:
                additive_nc
	|       relational_nc EQ_OP additive_nc
	|       relational_nc NE_OP additive_nc
	|       relational_nc LT_OP additive_nc
	|       relational_nc GT_OP additive_nc
	|       relational_nc LTEQ_OP additive_nc
	|       relational_nc GTEQ_OP additive_nc
                ;

logical_and_nc:
                relational_nc
	|       logical_and_nc AND_OP relational_nc
                ;

logical_or_nc:
                logical_and_nc
	|       logical_or_nc OR_OP logical_and_nc
                ;

conditional_nc:
                logical_or_nc
                ;

assignment_expression:
                conditional_expression
	|       unary_expression assignment_operator assignment_expression
                ;

assignment_operator:
                ASSIGN
        |       CASSIGN
	|       MUL_ASSIGN
	|       DIV_ASSIGN
	|       MOD_ASSIGN
	|       ADD_ASSIGN
	|       SUB_ASSIGN
	|       LEFT_ASSIGN
	|       RIGHT_ASSIGN
	|       AND_ASSIGN
	|       XOR_ASSIGN
	|       OR_ASSIGN
                ;
expression:     assignment_expression
	|       expression COMMA assignment_expression
                ;

constant_expression:
                conditional_expression
                ;

declaration:
                VAR local_var_spec
        |       VAR LPAREN local_var_specs RPAREN
        |       VAR LPAREN RPAREN
                ;

local_var_specs:
                local_var_spec
        |       local_var_specs SEMICOLON local_var_spec
        |       local_var_specs SEMICOLON
                ;

local_var_spec:
                var_names declaration_specifiers
                {
			varNamesRelease($1)
			Program.ReleaseCXArgBuilder($2)
                }
        |       var_names declaration_specifiers ASSIGN var_values
                {
			varNamesRelease($1)
			Program.ReleaseCXArgBuilder($2)
                }
        |       var_names ASSIGN var_values
                {
			varNamesRelease($1)
                }
                ;

// statements
statement:      labeled_statement
        |       compound_statement
	|       expression_statement
	|       selection_statement
	|       iteration_statement
        |       jump_statement
                ;

labeled_statement:
                IDENTIFIER COLON block_item
	|       CASE constant_expression COLON statement
	|       DEFAULT COLON statement
                ;

compound_statement:
                LBRACE block_item_list RBRACE
                ;

// Statements are separated by semicolons and a statement may be empty, as in
// Go, so the semicolon before a closing brace may be omitted
// (https://go.dev/ref/spec#Semicolons).
block_item_list:
                block_item
	|       block_item_list SEMICOLON block_item
                ;

block_item:
        |       declaration
        |       statement
        |       GLOBAL global_names
                ;

global_names:
                global_name
        |       global_names COMMA global_name
                ;

global_name:
                IDENTIFIER
        |       IDENTIFIER PERIOD IDENTIFIER
                ;

expression_statement:
                expression
                ;

selection_statement:
                IF conditional_nc compound_statement elseif_list else_statement
        |       IF conditional_nc compound_statement else_statement
        |       IF conditional_nc compound_statement elseif_list
        |       IF conditional_nc compound_statement
	|       SWITCH LPAREN conditional_expression RPAREN statement
                ;

elseif:         ELSE IF conditional_nc compound_statement
        ;

elseif_list:
                elseif
        |       elseif_list elseif
        ;

else_statement:
                ELSE compound_statement
        ;

iteration_statement:
                FOR conditional_nc compound_statement
        |       FOR for_clause_statement for_condition_statement compound_statement
        |       FOR for_clause_statement for_condition_statement for_clause_expression compound_statement
        |       FOR declaration SEMICOLON for_condition_statement compound_statement
        |       FOR declaration SEMICOLON for_condition_statement for_clause_expression compound_statement
                ;

// A for header holds no bare struct literal outside its init values
// (skycoin-cx#21).
for_clause_assignment:
                conditional_nc
	|       unary_nc assignment_operator for_clause_assignment
                ;

for_clause_expression:
                for_clause_assignment
	|       for_clause_expression COMMA for_clause_assignment
                ;

for_init_assignment:
                conditional_nc
	|       unary_nc assignment_operator assignment_expression
                ;

for_init_expression:
                for_init_assignment
	|       for_init_expression COMMA assignment_expression
                ;

for_clause_statement:
                SEMICOLON
	|       for_init_expression SEMICOLON
                ;

for_condition_statement:
                SEMICOLON
	|       for_clause_expression SEMICOLON
                ;

jump_statement: GOTO IDENTIFIER
	|       CONTINUE
	|       BREAK
	|       RETURN
	|       RETURN expression
                ;
%%
