package actions

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jinzhu/copier"
	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// SymbolsDataKey names a symbol of a scope by its package name and its name.
type SymbolsDataKey struct {
	SymbolsDataKeyPackage stringpool.StringGid
	SymbolsDataKeyName    stringpool.StringGid
}

type SymbolsData struct {
	varCount     int
	symbols      []ast.CXTypeSignatureIndex
	symbolsIndex []map[SymbolsDataKey]int
	// functionSymbols are the signatures lookupSymbol made for function names,
	// released with their arguments at the end of FunctionDeclaration.
	functionSymbols []ast.CXTypeSignatureIndex
	// escaped are the names of the locals that live in heap objects, and
	// escapedSlots the frame slots of their declarations.
	escaped      map[stringpool.StringGid]bool
	escapedSlots []escapeSlot
	// shortDeclarationNew is whether the := being declared has declared a
	// new name yet, see CXEXPR_SHORT_DECLARATION_SHARED.
	shortDeclarationNew bool
	// shortDeclarationFirst is the first name of that :=, where a := that
	// declares nothing is reported.
	shortDeclarationFirst ast.CXArgumentDebugGid
	shortDeclarationOpen  bool
}

// FunctionHeader takes a function name ('fnName') and either creates the
// function if it's not known before or returns the already existing function
// if it is.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		fnName - name of the function.
//	 receiver - the receiver of the method if the function is a method.
//				   i.e. func (sampleArg i32) SomeMethod(), sampleArg is the receiver.
//	 isMethod - If the function is a method (isMethod = true), then it adds the object that
//				   it's called on as the first argument.
//	 nameLine, nameColumn - the position of the name, which tells a second
//				   declaration of the name from the first, 0 when unknown.
func FunctionHeader(prgrm *ast.CXProgram, fnName string, receiver []*ast.CXArgument, isMethod bool, nameLine int, nameColumn int) ast.CXFunctionIndex {
	if isMethod {
		receiver = MethodReceiver(prgrm, receiver)
		if pkg, err := prgrm.GetCurrentPackage(); err == nil {
			receiverName := receiver[0].StructType.Name
			methodName := prgrm.CXProgramNameIntern(fnName)

			if fnIdx, ok := pkg.Methods[ast.CXPackageMethodKey{CXPackageMethodKeyStruct: receiverName, CXPackageMethodKeyMethod: methodName}]; ok {
				fnIdx = prgrm.FunctionDeclarationAt(fnIdx, CurrentFile, nameLine, nameColumn)
				fn := prgrm.GetFunctionFromArray(fnIdx)
				fn.AddInput(prgrm, receiver[0])
				pkg.CurrentFunction = fnIdx
				return fnIdx
			} else {
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now a constructor call, site S0723.
				fn := ast.MakeFunction(prgrm, prgrm.CXProgramMethodNameIntern(receiverName, methodName), CurrentFile, LineNo)
				fn.AddInput(prgrm, receiver[0])
				_, fnIdx := pkg.AddMethod(prgrm, fn, receiverName, methodName)
				pkg.CurrentFunction = fnIdx

				return fnIdx
			}
		} else {
			panic(err)
		}
	} else {
		if pkg, err := prgrm.GetCurrentPackage(); err == nil {
			fnNameGid := prgrm.CXProgramNameIntern(fnName)
			if fn, err := prgrm.GetFunction(fnNameGid, pkg.Name); err == nil {
				fnIdx := prgrm.FunctionDeclarationAt(ast.CXFunctionIndex(fn.Index), CurrentFile, nameLine, nameColumn)
				pkg.CurrentFunction = fnIdx
				return fnIdx
			} else {
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now a constructor call, site S0724.
				fn := ast.MakeFunction(prgrm, fnNameGid, CurrentFile, LineNo)
				_, fnIdx := pkg.AddFunction(prgrm, fn)
				pkg.CurrentFunction = fnIdx

				return fnIdx
			}
		} else {
			panic(err)
		}
	}
}

// MethodReceiver records a method with more than one receiver at the second
// and returns the first.
func MethodReceiver(prgrm *ast.CXProgram, receiver []*ast.CXArgument) []*ast.CXArgument {
	if len(receiver) <= 1 {
		return receiver
	}
	prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeMethodReceiversMultiple, receiver[1].ArgDetails, ast.CompileErrorOperands{})
	for _, extra := range receiver[1:] {
		prgrm.ReleaseCXArgBuilder(extra)
	}
	return receiver[:1]
}

// literalElementTypeCode is the error code of a literal element of the wrong
// type, written to the element variable name.
func literalElementTypeCode(prgrm *ast.CXProgram, name stringpool.StringGid) ast.CompileErrorCode {
	if bytes.HasPrefix(prgrm.CXProgramNameBytes(name), []byte(constants.ARRAY_ELEMENT_PREFIX)) {
		return ast.CompileErrorCodeArrayLiteralElementType
	}
	return ast.CompileErrorCodeSliceLiteralElementType
}

// assignValueTypeError is the error of a value of type inputType written to
// target of type outputType. The unnamed result of func f() T is a return.
func assignValueTypeError(prgrm *ast.CXProgram, inputType string, target stringpool.StringGid, outputType string) (ast.CompileErrorCode, ast.CompileErrorOperands) {
	if strings.HasPrefix(prgrm.CXProgramNameText(target), constants.RESULT_NAME) {
		return ast.CompileErrorCodeReturnValueType, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(inputType), prgrm.CXProgramCompileErrorText(outputType)}
	}
	return ast.CompileErrorCodeAssignValueType, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(inputType), int64(target), prgrm.CXProgramCompileErrorText(outputType)}
}

// FunctionResultUnnamed makes the result list of func f() T. The result
// gets a name no identifier can spell, so the body cannot refer to it.
func FunctionResultUnnamed(prgrm *ast.CXProgram, result *ast.CXArgument) []*ast.CXArgument {
	return FunctionResultsUnnamed(prgrm, []*ast.CXArgument{result})
}

// FunctionResultsUnnamed names the results of func f() (T, U) as
// FunctionResultUnnamed does, the first *result and the next *result1 on.
func FunctionResultsUnnamed(prgrm *ast.CXProgram, results []*ast.CXArgument) []*ast.CXArgument {
	pkg, err := prgrm.GetCurrentPackage()
	for i, result := range results {
		if err == nil {
			result.Package = ast.CXPackageIndex(pkg.Index)
		}
		name := constants.RESULT_NAME
		if i > 0 {
			name += strconv.Itoa(i)
		}
		result.Name = prgrm.CXProgramNameIntern(name)
	}
	return results
}

// FunctionAddParameters adds the function's input and output parameters.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		fnIdx - the index of the function in the main CXFunction array.
//	 inputs - parameters to be added in the function as input.
//	 outputs - parameters to be added in the function as output.
func FunctionAddParameters(prgrm *ast.CXProgram, fnIdx ast.CXFunctionIndex, inputs, outputs []*ast.CXArgument) {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0725.
	fn := prgrm.GetFunctionFromArray(fnIdx)

	fnInputs := fn.GetInputs(prgrm)
	if len(fnInputs) != len(inputs) {
		// it must be a method declaration
		// so we save the first input
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
		// Now a dropped object, site S0726.
		fn.Inputs.Fields = fn.Inputs.Fields[:1]
	} else {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
		// Now a dropped object, site S0727.
		fn.Inputs.Fields = nil
	}

	// we need to wipe the inputs recognized in the first pass
	// as these don't have all the fields correctly
	fn.Outputs = nil

	for _, inp := range inputs {
		fn.AddLocalVariableName(inp.Name)
		fn.AddInput(prgrm, inp)
	}

	for _, out := range outputs {
		fn.AddLocalVariableName(out.Name)
		fn.AddOutput(prgrm, out)
	}
}

func isParseOp(prgrm *ast.CXProgram, expr *ast.CXExpression) bool {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0728.
	exprAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0729.
	exprAtomicOpOperator := prgrm.GetFunctionFromArray(exprAtomicOp.Operator)
	if exprAtomicOpOperator != nil && exprAtomicOpOperator.AtomicOPCode > constants.START_PARSE_OPS && exprAtomicOpOperator.AtomicOPCode < constants.END_PARSE_OPS {
		return true
	}
	return false
}

// CheckUndValidTypes checks if an expression with a generic operator (operators that
// accept `cxcore.TYPE_UNDEFINED` arguments) is receiving arguments of valid types. For example,
// the expression `sa + sb` is not valid if they are struct instances.
func CheckUndValidTypes(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expressionIdx := exprs[currIndex].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0730.
	expressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[expressionIdx].Operator)
	if expressionOperator != nil && ast.IsOperator(expressionOperator.AtomicOPCode) && !IsAllArgsBasicTypes(prgrm, &exprs[currIndex]) {
		file, line := exprLine(prgrm, exprs, currIndex)
		prgrm.CXProgramCompileErrorAddLine(ast.CompileErrorCodeOperatorArgumentTypes, file, line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(ast.NativeNames.NativeNameIndexCodeText(expressionOperator.AtomicOPCode))})
	}
}

// ProcessFunctionParameters processes the scoping, offsets, and final size
// of the parameter args of the function.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		symbols - a slice of string-CXArg map which corresponds to
//				  the scoping of the  CXArguments. Each element in the slice
//				  corresponds to a different scope. The innermost scope is
//				  the last element of the slice.
//	 offset - offset to use by statements (excluding inputs, outputs
//				 and receiver).
//	 fnIdx - the index of the function in the main CXFunction array.
//	 params - function parameters to be processed.
func ProcessFunctionParameters(prgrm *ast.CXProgram, symbolsData *SymbolsData, offset *types.Pointer, fnIdx ast.CXFunctionIndex, params []ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0731.
	fn := prgrm.GetFunctionFromArray(fnIdx)

	for _, paramIdx := range params {
		UpdateSymbolsTable(prgrm, symbolsData, paramIdx, offset, false)
		// We remove it from local var array since it is already added to the symbols
		// if fn.IsLocalVariable(typeSignatureName) {
		// 	err := fn.RemoveLocalVariableFromArray(typeSignatureName)
		// 	if err != nil {
		// 		panic(err)
		// 	}
		// }

		GiveOffset(prgrm, symbolsData, paramIdx)
		AddPointer(prgrm, fn, paramIdx)

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0732.
		typeSig := prgrm.GetCXTypeSignatureFromArray(paramIdx)
		var param *ast.CXArgument
		if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0733.
			param = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))
		} else {
			continue
		}

		// as these are declarations, they should not have any dereference operations
		param.DereferenceOperations = nil
	}
}

// FunctionDeclaration completes the function's composition.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		fnIdx - the index of the function in the main CXFunction array.
//	 inputs - parameters to be added in the function as input.
//	 outputs - parameters to be added in the function as output.
//	 exprs - expression statements inside the function.
func FunctionDeclaration(prgrm *ast.CXProgram, fnIdx ast.CXFunctionIndex, inputs, outputs []*ast.CXArgument, exprs []ast.CXExpression) {
	// getting offset to use by statements (excluding inputs, outputs and receiver).
	var offset types.Pointer

	symIndex := make([]map[SymbolsDataKey]int, 0)
	symIndex = append(symIndex, make(map[SymbolsDataKey]int))

	symbolsData := &SymbolsData{
		varCount: 1,

		// symbols is a slice of string-int map which corresponds to
		// the scoping of the local variables. Each element in the slice
		// corresponds to a different scope. The innermost scope is
		// the last element of the slice.
		symbolsIndex: symIndex,
		symbols:      make([]ast.CXTypeSignatureIndex, 0),
	}

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	pkg.CurrentFunction = fnIdx
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0735.
	fn := prgrm.GetFunctionFromArray(fnIdx)

	scopeGlobalsCheckBegin(fnIdx)
	symbolsData.escaped, exprs = escapeFunction(prgrm, fn, inputs, outputs, exprs)
	FunctionAddParameters(prgrm, fnIdx, inputs, outputs)
	ProcessShortCircuitJumps(prgrm, exprs)
	ProcessGoTos(prgrm, exprs)
	ProcessLoopJumpsOutsideLoop(prgrm, exprs)
	AddExprsToFunction(prgrm, fnIdx, exprs)
	// The steps above may grow prgrm.CXFunctions, which moves fn.
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, as at site S0735.
	fn = prgrm.GetFunctionFromArray(fnIdx)
	ProcessFunctionParameters(prgrm, symbolsData, &offset, fnIdx, fn.GetInputs(prgrm))
	ProcessFunctionParameters(prgrm, symbolsData, &offset, fnIdx, fn.GetOutputs(prgrm))

	for i, expr := range fn.Expressions {
		if expr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0736.
		exprAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		for s := uint8(0); s < expr.ExpressionScopeOpenCount; s++ {
			symbolsData.symbolsIndex = append(symbolsData.symbolsIndex, make(map[SymbolsDataKey]int))
		}

		if !ProcessMethodCall(prgrm, &expr, symbolsData) {
			// Only the results are declared, as types.INVALID placeholders.
			ProcessExpressionArguments(prgrm, symbolsData, &offset, fnIdx, exprAtomicOp.GetOutputs(prgrm), &expr, false)
			symbolsScopeClose(symbolsData, expr)
			continue
		}

		incDec := incDecOperandMissing(prgrm, exprAtomicOp)
		ProcessExpressionArguments(prgrm, symbolsData, &offset, fnIdx, exprAtomicOp.GetInputs(prgrm), &expr, true)
		if incDec {
			incDecOperandAdd(prgrm, exprAtomicOp)
		}
		ProcessExpressionArguments(prgrm, symbolsData, &offset, fnIdx, exprAtomicOp.GetOutputs(prgrm), &expr, false)
		checkOperatorSameType(prgrm, fn.Expressions, i)

		ProcessPointerStructs(prgrm, &expr)
		ProcessTempVariable(prgrm, &expr)
		ProcessSliceAssignment(prgrm, &expr)
		ProcessStringAssignment(prgrm, &expr)
		// ProcessReferenceAssignment(prgrm, &expr)
		ProcessShortDeclaration(prgrm, &expr, fn.Expressions, i, &offset)
		processTestExpression(prgrm, fn.Expressions, i)
		if expressionOperandInvalid(prgrm, exprAtomicOp) {
			// A placeholder of a reported error, so its type checks would only repeat it.
			symbolsScopeClose(symbolsData, expr)
			continue
		}

		if !CheckTypes(prgrm, fn.Expressions, i) {
			symbolsScopeClose(symbolsData, expr)
			continue
		}
		CheckUndValidTypes(prgrm, fn.Expressions, i)
		checkOperatorOperands(prgrm, fn.Expressions, i)
		checkOperatorResultType(prgrm, fn.Expressions, i)
		checkSliceBuiltinTypes(prgrm, fn.Expressions, i)
		ProcessTypedOperator(prgrm, fn.Expressions, i)
		symbolsScopeClose(symbolsData, expr)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, as at site S0735.
	fn = prgrm.GetFunctionFromArray(fnIdx)
	for _, slot := range symbolsData.escapedSlots {
		gcRootAddEscapeSlot(fn, slot.escapeSlotOffset)
	}
	fn.LineCount = len(fn.Expressions)
	fn.Size = offset

	releaseFunctionSymbols(prgrm, symbolsData)
	scopeGlobalsEnd(prgrm)
}

// symbolsScopeClose closes the scopes that expr ends.
func symbolsScopeClose(symbolsData *SymbolsData, expr ast.CXExpression) {
	assert.AssertInvariantTrue(len(symbolsData.symbolsIndex) > int(expr.ExpressionScopeCloseCount), "FunctionDeclaration: scope close without a matching open")
	symbolsData.symbolsIndex = symbolsData.symbolsIndex[:len(symbolsData.symbolsIndex)-int(expr.ExpressionScopeCloseCount)]
}

// ProcessTypedOperator gets the proper typed operator for the expression.
// i.e. add -> i32.add
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	exprs, currIndex - the expressions and the index of the one to process.
func ProcessTypedOperator(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expr := &exprs[currIndex]
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0737.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0738.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	if expressionOperator == nil {
		return
	}
	if ast.IsOperator(expressionOperator.AtomicOPCode) {
		atomicType := operatorOperandType(prgrm, expression.GetInputs(prgrm)[0])

		typedOp := ast.GetTypedOperator(atomicType, expressionOperator.AtomicOPCode)
		if typedOp == nil {
			if ast.OpcodeHandlers[expressionOperator.AtomicOPCode] == nil {
				file, line := exprLine(prgrm, exprs, currIndex)
				prgrm.CXProgramCompileErrorAddLine(ast.CompileErrorCodeOperatorArgumentTypes, file, line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(ast.NativeNames.NativeNameIndexCodeText(expressionOperator.AtomicOPCode))})
			}
			return
		}

		expressionOperator.AtomicOPCode = typedOp.AtomicOPCode

	}
}

// operatorOperandType returns the type an operator dispatches on for one
// operand. A pointer that is not dereferenced dispatches as types.POINTER.
func operatorOperandType(prgrm *ast.CXProgram, sigIdx ast.CXTypeSignatureIndex) types.Code {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0739.
	sig := prgrm.GetCXTypeSignatureFromArray(sigIdx)
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		if sig.Type == ast.TYPE_POINTER_ATOMIC {
			return types.POINTER
		}
		return sig.GetType(prgrm)
	}

	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S0740.
	arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta))
	if declaredAsPointer(arg.GetAssignmentElement(prgrm).DeclarationSpecifiers) {
		return types.POINTER
	}
	return arg.GetType(prgrm)
}

// declaredAsPointer reports whether the outermost declaration specifier left
// after dereferences and indexing is a pointer.
func declaredAsPointer(specs []int) bool {
	var stack []int
	for _, spec := range specs {
		switch spec {
		case constants.DECL_DEREF, constants.DECL_INDEXING:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, spec)
		}
	}
	return len(stack) > 0 && stack[len(stack)-1] == constants.DECL_POINTER
}

// pointerIndexedBeforeDeref reports whether specs index a pointer before the
// first dereference, as in p[i] or *p[i] on a *[]T.
func pointerIndexedBeforeDeref(specs []int) bool {
	for _, spec := range specs {
		switch spec {
		case constants.DECL_DEREF:
			return false
		case constants.DECL_INDEXING:
			return true
		}
	}
	return false
}

var operatorSymbols = map[int]string{
	constants.OP_EQUAL:    "==",
	constants.OP_UNEQUAL:  "!=",
	constants.OP_LT:       "<",
	constants.OP_GT:       ">",
	constants.OP_LTEQ:     "<=",
	constants.OP_GTEQ:     ">=",
	constants.OP_BITAND:   "&",
	constants.OP_BITOR:    "|",
	constants.OP_BITXOR:   "^",
	constants.OP_BITCLEAR: "&^",
	constants.OP_BITSHL:   "<<",
	constants.OP_BITSHR:   ">>",
	constants.OP_MUL:      "*",
	constants.OP_DIV:      "/",
	constants.OP_MOD:      "%",
	constants.OP_ADD:      "+",
	constants.OP_SUB:      "-",
	constants.OP_NEG:      "-",
}

// checkOperatorOperands rejects the operators that language revision 1
// removed: every operator on str, and ordering on pointers.
func checkOperatorOperands(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expression, err := prgrm.GetCXAtomicOpFromExpressions(exprs, currIndex)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0741.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	if expressionOperator == nil || !ast.IsOperator(expressionOperator.AtomicOPCode) || len(expression.GetInputs(prgrm)) == 0 {
		return
	}

	opCode := expressionOperator.AtomicOPCode
	sym := operatorSymbols[opCode]
	code := ast.CompileErrorCodeNil
	switch operatorOperandType(prgrm, expression.GetInputs(prgrm)[0]) {
	case types.STR:
		switch opCode {
		case constants.OP_EQUAL, constants.OP_UNEQUAL:
			code = ast.CompileErrorCodeStrEqualityOperator
		case constants.OP_LT, constants.OP_GT, constants.OP_LTEQ, constants.OP_GTEQ:
			code = ast.CompileErrorCodeStrOrderingOperator
		case constants.OP_ADD:
			code = ast.CompileErrorCodeStrConcatenation
		default:
			code = ast.CompileErrorCodeStrArithmeticOperator
		}
	case types.POINTER:
		switch opCode {
		case constants.OP_LT, constants.OP_GT, constants.OP_LTEQ, constants.OP_GTEQ:
			code = ast.CompileErrorCodePointerOrderingOperator
		}
	}
	if code == ast.CompileErrorCodeNil {
		return
	}

	file, line := exprLine(prgrm, exprs, currIndex)
	column := prgrm.CXProgramCompileErrorColumn(file, line)
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0742.
	if sig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0]); sig.ArgDetails != ast.CXArgumentDebugGidNil {
		if sigFile, sigLine, sigColumn := ast.CXArgumentDebugPosition(prgrm, sig.ArgDetails); sigLine > 0 {
			file, line, column = sigFile, sigLine, sigColumn
		}
	}
	prgrm.CXProgramCompileErrorAddPosition(code, file, line, column, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(sym)})
}

// checkOperatorResultType rejects an arithmetic or bitwise result stored in
// a variable of another type, as in var f f32 = -1 (no implicit casting).
func checkOperatorResultType(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expression, err := prgrm.GetCXAtomicOpFromExpressions(exprs, currIndex)
	if err != nil {
		panic(err)
	}
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	if expressionOperator == nil || !ast.IsOperator(expressionOperator.AtomicOPCode) || ast.IsComparisonOperator(expressionOperator.AtomicOPCode) {
		return
	}
	inputs := expression.GetInputs(prgrm)
	outputs := expression.GetOutputs(prgrm)
	if len(inputs) == 0 || len(outputs) != 1 {
		return
	}
	outputTypeSig := prgrm.GetCXTypeSignatureFromArray(outputs[0])
	if IsTempVar(prgrm, outputTypeSig.Name) || typeSignatureUndefined(prgrm, outputTypeSig) {
		return
	}
	inputTypeSig := prgrm.GetCXTypeSignatureFromArray(inputs[0])
	if typeSignatureUndefined(prgrm, inputTypeSig) || ast.GetTypedOperator(operatorOperandType(prgrm, inputs[0]), expressionOperator.AtomicOPCode) == nil {
		return
	}
	inputType := ast.GetFormattedType(prgrm, inputTypeSig)
	outputType := ast.GetFormattedType(prgrm, outputTypeSig)
	if inputType == outputType {
		return
	}
	outputName := outputTypeSig.Name
	if outputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		outputName = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outputTypeSig.Meta)).GetAssignmentElement(prgrm).Name
	}
	file, line := exprLine(prgrm, exprs, currIndex)
	column := prgrm.CXProgramCompileErrorColumn(file, line)
	if sigFile, sigLine, sigColumn := ast.CXArgumentDebugPosition(prgrm, outputTypeSig.ArgDetails); sigLine > 0 {
		file, line, column = sigFile, sigLine, sigColumn
	} else if sigFile, sigLine, sigColumn := ast.CXArgumentDebugPosition(prgrm, inputTypeSig.ArgDetails); sigLine > 0 {
		file, line, column = sigFile, sigLine, sigColumn
	}
	code, operands := assignValueTypeError(prgrm, inputType, StripNameNumber(prgrm, outputName), outputType)
	prgrm.CXProgramCompileErrorAddPosition(code, file, line, column, operands)
}

// FunctionCall completes the array of expressions for postfix function call.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		exprs - the array of expressions.
//	 arguments - the expressions of each input argument of the function.
func FunctionCall(prgrm *ast.CXProgram, exprs []ast.CXExpression, arguments [][]ast.CXExpression) []ast.CXExpression {
	expr := exprs[len(exprs)-1]

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0743.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0744.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	if expressionOperator == nil {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0745.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		// A possible method call has fields, an atomic output has none.
		fieldless := true
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0747.
			fieldless = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta)).Fields == nil
		}

		opName := expressionOutputTypeSig.Name
		opPkgIdx := expressionOutputTypeSig.Package

		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0749.
		opPkg, err := prgrm.GetPackageFromArray(opPkgIdx)
		if err != nil {
			panic(err)
		}

		if op, err := prgrm.GetFunction(opName, opPkg.Name); err == nil && fieldless {
			expression.Operator = ast.CXFunctionIndex(op.Index)
		} else if fieldless {
			// then it's not a possible method call
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeMessage, CurrentFile, LineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})
			expression.Operator = invalidFunctionIndex(prgrm, opPkgIdx, opName)
		} else {
			// The receiver names the method as its last field and is the first input.
			exprs[len(exprs)-1].ExpressionType = ast.CXEXPR_METHOD_CALL
			receiverIdx := expression.GetOutputs(prgrm)[0]
			arguments := expression.GetInputs(prgrm)
			expression.Outputs = nil
			expression.Inputs = nil
			expression.AddInput(prgrm, receiverIdx)
			for _, argumentIdx := range arguments {
				expression.AddInput(prgrm, argumentIdx)
			}
		}

		if len(expression.GetOutputs(prgrm)) > 0 && fieldless {
			expression.Outputs = nil
		}
	}
	if !exprs[len(exprs)-1].IsMethodCall() {
		exprs[len(exprs)-1].ExpressionType = ast.CXEXPR_FUNCTION_CALL
	}

	args, argumentLast := functionCallArguments(arguments)
	var nestedExprs []ast.CXExpression
	for i, inpExpr := range args {
		if inpExpr.Type == ast.CX_LINE {
			continue
		}

		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0750.
		inpExprAtomicOp, err := prgrm.GetCXAtomicOp(inpExpr.Index)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0751.
		inpExprAtomicOpOperator := prgrm.GetFunctionFromArray(inpExprAtomicOp.Operator)

		inpExprCXLine, err := prgrm.GetPreviousCXLine(args, i)
		if err != nil {
			// An argument such as -1 has no line of its own.
			inpExprCXLine = &ast.CXLine{LineNumber: LineNo}
		}
		if inpExprAtomicOpOperator == nil && !argumentLast[i] && !inpExpr.IsMethodCall() {
			// A declaration inside an argument, such as the variable of a literal.
			nestedExprs = append(nestedExprs, inpExpr)
			continue
		}

		if inpExprAtomicOpOperator != nil && inpExprAtomicOpOperator.AtomicOPCode == constants.OP_JMP {
			// A short-circuit jmp of && or || inside an argument.
			nestedExprs = append(nestedExprs, inpExpr)
			continue
		}
		if inpExprAtomicOpOperator == nil && !inpExpr.IsMethodCall() {
			typeSigIdx := inpExprAtomicOp.GetOutputs(prgrm)[0]

			// typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)

			// // TODO: This is still to be improved
			// // Get its CX argument if its type cx arg deprecate
			// var typeSigArg *ast.CXArgument = &ast.CXArgument{}
			// if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// 	typeSigArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))

			// 	// If an atomic type, convert cx type signature to type atomic/
			// 	if ast.IsTypeAtomic(typeSigArg) {
			// 		typeSig.Name = typeSigArg.Name
			// 		typeSig.Type = ast.TYPE_ATOMIC
			// 		typeSig.Meta = int(typeSigArg.Type)
			// 		typeSig.Offset = typeSigArg.Offset
			// 		typeSig.Package = typeSigArg.Package
			// 	}
			// } else if typeSig.Type == ast.TYPE_ATOMIC || typeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// 	// do nothing
			// }

			// then it's a literal
			expression.AddInput(prgrm, typeSigIdx)
		} else {
			// then it's a function call
			outputGiven := len(inpExprAtomicOp.GetOutputs(prgrm)) > 0
			if !outputGiven && len(inpExprAtomicOpOperator.GetOutputs(prgrm)) == 0 {
				inpExprAtomicOpOperator = outputlessCallInvalidate(prgrm, inpExprAtomicOp, inpExprAtomicOpOperator, CurrentFile, inpExprCXLine.LineNumber)
			}
			if len(inpExprAtomicOp.GetOutputs(prgrm)) < 1 {
				var out *ast.CXArgument

				inpExprAtomicOpOperatorOutputs := inpExprAtomicOpOperator.GetOutputs(prgrm)
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0753.
				inpExprAtomicOpOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(inpExprAtomicOpOperatorOutputs[0])
				var inpExprAtomicOpOperatorOutputArg *ast.CXArgument
				if inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0755.
					inpExprAtomicOpOperatorOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inpExprAtomicOpOperatorOutputTypeSig.Meta))

					var typeSigIdx ast.CXTypeSignatureIndex

					// if undefined type, then adopt argument's type
					if inpExprAtomicOpOperatorOutputArg.Type == types.UNDEFINED {
						// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
						// Now an arena getter, site S0756.
						inpExprAtomicOpInputTypeSig := prgrm.GetCXTypeSignatureFromArray(inpExprAtomicOp.GetInputs(prgrm)[0])

						var inpExprAtomicOpInputArg *ast.CXArgument
						if inpExprAtomicOpInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
							// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
							// Now an arena getter, site S0758.
							inpExprAtomicOpInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inpExprAtomicOpInputTypeSig.Meta))

							_, out = ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, inpExprCXLine.LineNumber)
							if len(inpExprAtomicOpInputArg.Fields) == 0 {
								out.SetType(inpExprAtomicOpInputArg.Type)
								out.StructType = inpExprAtomicOpInputArg.StructType
								out.Size = inpExprAtomicOpInputArg.Size
								out.Type = inpExprAtomicOpInputArg.Type
								out.PointerTargetType = inpExprAtomicOpInputArg.PointerTargetType
							} else {
								// T{a: 1}.a is typed by its field, which is resolved later.
								out.SetType(types.UNDEFINED)
							}
							out.PreviouslyDeclared = true

							out.Package = inpExprAtomicOp.Package

							typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
						} else if inpExprAtomicOpInputTypeSig.Type == ast.TYPE_ATOMIC || inpExprAtomicOpInputTypeSig.Type == ast.TYPE_SLICE_ATOMIC && operandIndexed(prgrm, inpExprAtomicOpInputTypeSig) {
							argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
							argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
							argDetails.FileName = CurrentFile
							argDetails.FileLine = inpExprCXLine.LineNumber
							argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
							_, outTypeSig := prgrm.AllocCXTypeSignature()
							outTypeSig.ArgDetails = argDetailsGid
							outTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
							outTypeSig.Type = ast.TYPE_ATOMIC
							outTypeSig.Meta = inpExprAtomicOpInputTypeSig.Meta
							if inpExprAtomicOpInputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
								// An element of the slice placeholder of f()[i].
								outTypeSig.Meta = prgrm.GetCXTypeSignatureArrayFromArray(inpExprAtomicOpInputTypeSig.Meta).Meta
							}
							outTypeSig.Package = inpExprAtomicOpInputTypeSig.Package
							outTypeSig.Offset = inpExprAtomicOpInputTypeSig.Offset

							typeSigIdx = outTypeSig.Index
						} else if inpExprAtomicOpInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
							argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
							argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
							argDetails.FileName = CurrentFile
							argDetails.FileLine = inpExprCXLine.LineNumber
							argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
							_, outTypeSig := prgrm.AllocCXTypeSignature()
							outTypeSig.ArgDetails = argDetailsGid
							outTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
							outTypeSig.Type = ast.TYPE_POINTER_ATOMIC
							outTypeSig.Meta = inpExprAtomicOpInputTypeSig.Meta
							outTypeSig.Package = inpExprAtomicOpInputTypeSig.Package
							outTypeSig.Offset = inpExprAtomicOpInputTypeSig.Offset

							typeSigIdx = outTypeSig.Index
						} else {
							panic("type is not known")
						}

					} else {
						_, out = ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, inpExprCXLine.LineNumber)
						argumentTypeCopy(out, inpExprAtomicOpOperatorOutputArg)
						out.PassBy = constants.PASSBY_VALUE

						if specs := inpExprAtomicOpOperatorOutputArg.DeclarationSpecifiers; inpExprAtomicOpOperatorOutputArg.StructType != nil && (len(specs) < 2 || specs[1] != constants.DECL_POINTER) {
							// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
							// Now an arena getter, site S0768.
							inpExprPkg, err := prgrm.GetPackageFromArray(inpExprAtomicOp.Package)
							if err != nil {
								panic(err)
							}
							if strct, err := inpExprPkg.GetStruct(prgrm, inpExprAtomicOpOperatorOutputArg.StructType.Name); err == nil {
								out.Size = strct.GetStructSize(prgrm)
							}
						}
						out.PreviouslyDeclared = true
						out.Package = inpExprAtomicOp.Package

						typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
					}

					inpExprAtomicOp.AddOutput(prgrm, typeSigIdx)
					expression.AddInput(prgrm, typeSigIdx)
				} else if inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC || inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC || inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
					_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(inpExprAtomicOpOperatorOutputTypeSig)
					newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
					newTypeSig.Package = inpExprAtomicOp.Package
					newTypeSigIdx := newTypeSig.Index

					inpExprAtomicOp.AddOutput(prgrm, newTypeSigIdx)
					expression.AddInput(prgrm, newTypeSigIdx)
				} else if inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(inpExprAtomicOpOperatorOutputTypeSig)
					newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
					newTypeSig.Package = inpExprAtomicOp.Package
					newTypeSigIdx := newTypeSig.Index

					inpExprAtomicOp.AddOutput(prgrm, newTypeSigIdx)
					expression.AddInput(prgrm, newTypeSigIdx)
				} else if inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_STRUCT || inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_POINTER_STRUCT || typeSignatureCollection(inpExprAtomicOpOperatorOutputTypeSig) {
					_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(inpExprAtomicOpOperatorOutputTypeSig)
					newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
					newTypeSig.Package = inpExprAtomicOp.Package
					newTypeSigIdx := newTypeSig.Index

					inpExprAtomicOp.AddOutput(prgrm, newTypeSigIdx)
					expression.AddInput(prgrm, newTypeSigIdx)
				} else {
					panic("type is not known")
				}

			}
			if argumentLast[i] && outputGiven {
				// The value of the argument is already in an output, as for a literal.
				typeSig := inpExprAtomicOp.GetOutputs(prgrm)[0]
				expression.AddInput(prgrm, typeSig)
			}
			nestedExprs = append(nestedExprs, inpExpr)
		}
	}

	return append(nestedExprs, exprs...)
}

// checkSameNativeType checks if all the inputs of an expression are of the same type.
// It is used mainly to prevent implicit castings in arithmetic operations
func checkSameNativeType(prgrm *ast.CXProgram, expr *ast.CXExpression) error {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0776.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	if len(expression.GetInputs(prgrm)) < 1 {
		return errors.New("cannot perform arithmetic without operands")
	}

	var typeCode types.Code
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0777.
	expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
	if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0778.
		expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

		typeCode = expressionInputArg.Type
		if expressionInputArg.Type == types.POINTER {
			typeCode = expressionInputArg.PointerTargetType
		}
	} else {
		typeCode = expressionInputTypeSig.GetType(prgrm)
	}

	for _, inputIdx := range expression.GetInputs(prgrm) {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0779.
		input := prgrm.GetCXTypeSignatureFromArray(inputIdx)

		var inpType types.Code
		if input.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0780.
			inp := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(input.Meta))

			inpType = inp.Type
			if inp.Type == types.POINTER {
				inpType = inp.PointerTargetType
			}
		} else {
			inpType = input.GetType(prgrm)
		}

		if inpType != typeCode {
			return fmt.Errorf("operands are not of the same type: %s != %s", inpType.Name(), typeCode.Name())
		}

		typeCode = inpType
	}

	return nil
}

// ProcessOperatorExpression sizes the untyped outputs of an operator from its
// inputs. checkOperatorSameType checks that the inputs have one type.
func ProcessOperatorExpression(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0781.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0782.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	if expr.IsUndType(prgrm) {
		for _, outputIdx := range expression.GetOutputs(prgrm) {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0783.
			output := prgrm.GetCXTypeSignatureFromArray(outputIdx)

			var outIdx ast.CXArgumentIndex
			if output.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				outIdx = ast.CXArgumentIndex(output.Meta)

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0784.
				out := prgrm.GetCXArgFromArray(outIdx)

				size := types.Pointer(1)
				if !ast.IsComparisonOperator(expressionOperator.AtomicOPCode) {
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S0785.
					expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])

					var expressionInputArg *ast.CXArgument
					if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S0787.
						expressionInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

						size = ast.GetArgSize(prgrm, expressionInputArg.GetAssignmentElement(prgrm))
					} else {
						size = expressionInputTypeSig.GetSize(prgrm, false)
					}
				}

				out.Size = size
			} else if output.Type == ast.TYPE_ATOMIC || output.Type == ast.TYPE_POINTER_ATOMIC {
				continue
			} else if output.Type == ast.TYPE_STRUCT {
				continue
			} else {
				panic("type is not known")
			}
		}
	}
}

// ProcessPointerStructs checks and processes if any args is a pointer struct.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	expr - the operator expression.
func ProcessPointerStructs(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0788.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	for _, argTypeSigIdx := range append(expression.GetInputs(prgrm), expression.GetOutputs(prgrm)...) {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0789.
		argTypeSig := prgrm.GetCXTypeSignatureFromArray(argTypeSigIdx)

		var arg *ast.CXArgument
		if argTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0791.
			arg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argTypeSig.Meta))
		} else if argTypeSig.Type == ast.TYPE_ATOMIC || argTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_POINTER_ARRAY_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_STRUCT {
			// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
			// Now an arena getter.
			pointerFieldsDeref(prgrm, prgrm.GetCXTypeSignatureStructFromArray(argTypeSig.Meta).Fields)
			continue
		} else if argTypeSig.Type == ast.TYPE_POINTER_STRUCT {
			// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
			// Now an arena getter, site S0792.
			structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSig.Meta)
			if len(structDetails.Fields) > 0 {
				argTypeSig.IsDeref = true
				pointerFieldsDeref(prgrm, structDetails.Fields)
			}
			continue
		} else {
			panic("type is not known")
		}

		pointerFieldsDeref(prgrm, arg.Fields)

		doesArgHaveDerefPointer := false
		for _, deref := range arg.DereferenceOperations {
			if deref == constants.DEREF_POINTER {
				doesArgHaveDerefPointer = true
			}
		}
		if arg.IsStruct() && arg.IsPointer() && len(arg.Fields) > 0 && !doesArgHaveDerefPointer && !pointerTypeOfLastField(prgrm, arg) {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1679.
			prgrm.GetCXArg(ast.CXArgumentIndex(arg.Index)).DereferenceOperations = append(arg.DereferenceOperations, constants.DEREF_POINTER)
		}
	}
}

// pointerFieldsDeref makes a pointer field read through the pointer when another
// field follows, so a.p.x reads x of *a.p while a.p stays the pointer.
func pointerFieldsDeref(prgrm *ast.CXProgram, fields []ast.CXArgumentIndex) {
	for _, fldIdx := range fields[:max(len(fields)-1, 0)] {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0793.
		fld := prgrm.GetCXArgFromArray(fldIdx)
		if fld.IsPointer() && !hasDerefOp(fld, constants.DEREF_POINTER) {
			fld.DereferenceOperations = append(fld.DereferenceOperations, constants.DEREF_POINTER)
		}
	}
}

// processTestExpression checks for the special case of test calls. `assert`, `test`, `panic` are operators where
// their first input's type needs to be the same as its second input's type. This can't be handled by
// `checkSameNativeType` because these test functions' third input parameter is always a `str`.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	exprs - the function's expressions.
//	currIndex - the index of the operator expression in exprs.
func processTestExpression(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0795.
	expression, err := prgrm.GetCXAtomicOp(exprs[currIndex].Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0796.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	if expressionOperator != nil {
		opCode := expressionOperator.AtomicOPCode
		if opCode == constants.OP_ASSERT || opCode == constants.OP_TEST || opCode == constants.OP_PANIC {
			var inp1Type, inp2Type string

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0797.
			expressionInputFirstTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
			inp1Type = ast.GetFormattedType(prgrm, expressionInputFirstTypeSig)

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0798.
			expressionInputSecondTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[1])
			inp2Type = ast.GetFormattedType(prgrm, expressionInputSecondTypeSig)
			if inp1Type != inp2Type {
				file, line := exprLine(prgrm, exprs, currIndex)
				prgrm.CXProgramCompileErrorAddLine(ast.CompileErrorCodeInputTypesNotEqual, file, line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(ast.NativeNames.NativeNameIndexCodeText(expressionOperator.AtomicOPCode)), prgrm.CXProgramCompileErrorText(inp1Type), prgrm.CXProgramCompileErrorText(inp2Type)})
			}
		}
	}
}

// checkIndexType throws an error if the type of `idx` is not `i32` or `i64`.
func checkIndexType(prgrm *ast.CXProgram, idxIdx ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0799.
	idxTypeSig := prgrm.GetCXTypeSignatureFromArray(idxIdx)
	var idxType string

	idxType = ast.GetFormattedType(prgrm, idxTypeSig)
	if idxType != "i32" && idxType != "i64" {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeIndexType, idxTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(idxType)})
	}
}

// ProcessExpressionArguments performs a series of checks and processes to an expresion's inputs and outputs.
// Some of these checks are: checking if a an input has not been declared, assign a relative offset to the argument,
// and calculate the correct size of the argument.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		symbols - a slice of string-CXArg map which corresponds to
//				  the scoping of the  CXArguments. Each element in the slice
//				  corresponds to a different scope. The innermost scope is
//				  the last element of the slice.
//	 offset - offset to use by statements (excluding inputs, outputs
//				 and receiver).
//	 fnIdx - the index of the function in the main CXFunction array.
//	 args - the expression arguments.
//	 expr - the expression.
//	 isInput - true if args are input arguments, false if they are output args.
func ProcessExpressionArguments(prgrm *ast.CXProgram, symbolsData *SymbolsData, offset *types.Pointer, fnIdx ast.CXFunctionIndex, args []ast.CXTypeSignatureIndex, expr *ast.CXExpression, isInput bool) {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0800.
	fn := prgrm.GetFunctionFromArray(fnIdx)

	for _, typeSignatureIdx := range args {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0801.
		typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
		var argIdx ast.CXArgumentIndex
		if typeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			argIdx = ast.CXArgumentIndex(typeSignature.Meta)
		} else {
			argIdx = -1
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0802.
		arg := prgrm.GetCXArgFromArray(argIdx)

		if !isInput {
			CheckRedeclared(prgrm, symbolsData, expr, typeSignatureIdx)
		}

		if !isInput {
			ProcessOperatorExpression(prgrm, expr)
		}

		// TODO: Check how to remove PreviouslyDeclared field
		// maybe its only used by temp variables
		// TempVar always have PreviouslyDeclared=true
		typeSignatureName := typeSignature.Name
		isLocalVar := fn.IsLocalVariable(typeSignatureName)
		if (arg != nil && arg.PreviouslyDeclared) || IsTempVar(prgrm, typeSignatureName) || (arg == nil && isLocalVar) {
			UpdateSymbolsTable(prgrm, symbolsData, typeSignatureIdx, offset, false)
		} else {
			UpdateSymbolsTable(prgrm, symbolsData, typeSignatureIdx, offset, true)
		}

		GiveOffset(prgrm, symbolsData, typeSignatureIdx)
		ProcessSlice(prgrm, argIdx)

		if arg != nil {
			for _, idxIdx := range arg.Indexes {
				UpdateSymbolsTable(prgrm, symbolsData, idxIdx, offset, true)
				GiveOffset(prgrm, symbolsData, idxIdx)
				checkIndexType(prgrm, idxIdx)
			}
			for _, fldIdx := range arg.Fields {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0803.
				fld := prgrm.GetCXArgFromArray(fldIdx)
				for _, idxIdx := range fld.Indexes {
					UpdateSymbolsTable(prgrm, symbolsData, idxIdx, offset, true)
					GiveOffset(prgrm, symbolsData, idxIdx)
				}
			}
		} else if typeSignature.Type == ast.TYPE_SLICE_ATOMIC {
			// The slice placeholder of f()[i] keeps its indexes in its details.
			for _, idxIdx := range prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta).Indexes {
				UpdateSymbolsTable(prgrm, symbolsData, idxIdx, offset, true)
				GiveOffset(prgrm, symbolsData, idxIdx)
				checkIndexType(prgrm, idxIdx)
			}
		}
		AddPointer(prgrm, fn, typeSignatureIdx)
	}
}

// isPointerAdded checks if `sym` has already been added to `fn.ListOfPointers`.
// GCREACH: reads the GC root table ListOfPointers,
// see docs/GarbageCollector.md.
func isPointerAdded(prgrm *ast.CXProgram, fn *ast.CXFunction, sym *ast.CXArgument) (found bool) {
	for _, ptr := range fn.ListOfPointers {
		if sym.Name == ptr.Name {
			if len(sym.Fields) == 0 && len(ptr.Fields) == 0 {
				found = true
				break
			}

			// Checking if we're referring to the same symbol and
			// same fields being accessed. For instance, `foo.bar` !=
			// `foo.car` as these will have different memory offset to
			// be considered by the garbage collector.
			if len(sym.Fields) > 0 &&
				len(sym.Fields) == len(ptr.Fields) &&
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now arena getters, sites S1680 S1681.
				prgrm.GetCXArg(ast.CXArgumentIndex(sym.Fields[len(sym.Fields)-1])).Name == prgrm.GetCXArg(ast.CXArgumentIndex(ptr.Fields[len(ptr.Fields)-1])).Name {
				found = true
				break
			}
		}
	}

	return found
}

// AddPointer checks if `sym` or its last field, if a struct, behaves like a
// pointer (slice, pointer, string). If this is the case, `sym` is added to
// `fn.ListOfPointers` so the CX runtime does not have to determine this.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		fn - the function the sym belongs.
//	 typeSigIdx - the index of the type signature from the main CXTypeSignature array.
//
// GCREACH: adds GC roots to ListOfPointers,
// see docs/GarbageCollector.md.
func AddPointer(prgrm *ast.CXProgram, fn *ast.CXFunction, typeSigIdx ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0804.
	typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)
	var sym *ast.CXArgument
	if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0806.
		sym = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))
	} else {
		return
	}

	// Ignore if it's a global variable.
	if sym.Offset >= prgrm.Stack.Size && !ast.OffsetEscaped(sym.Offset) {
		return
	}
	// We first need to check if we're going to add `sym` with fields.
	// If `sym` has fields, then we `return` and we don't add the root `sym`.
	// If `sym` has no fields, then we check if `sym` is a pointer and
	// we add it if it is.

	// Field symbol:
	// Checking if it is a pointer candidate and if it was already
	// added to the list.
	if len(sym.Fields) > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0807.
		field := prgrm.GetCXArgFromArray(sym.Fields[len(sym.Fields)-1])
		if field.IsPointer() && !isPointerAdded(prgrm, fn, sym) {
			fn.ListOfPointers = append(fn.ListOfPointers, sym)
		}
	}
	// Root symbol:
	// Checking if it is a pointer candidate and if it was already
	// added to the list.
	if sym.IsPointer() && !isPointerAdded(prgrm, fn, sym) {
		if len(sym.Fields) > 0 {
			// A copy of sym without its fields, owned by fn.
			_, tmp := prgrm.AllocCXArgument()
			tmpIndex := tmp.Index
			copier.Copy(tmp, sym)
			tmp.Index = tmpIndex
			// OBJMODEL: state.CXArgumentMng.CXArgumentRelease(gid), phase 4 #326.
			// Now a dropped object, site S0809.
			tmp.Fields = nil
			fn.ListOfPointers = append(fn.ListOfPointers, tmp)
		} else {
			fn.ListOfPointers = append(fn.ListOfPointers, sym)
		}
	}
}

// fieldPointerDeref dereferences sym once more when its explicit
// dereferences and indexes leave a pointer to a struct, as (*pp).f does for
// a **T pp and (*s[0]).f for a []**T s.
func fieldPointerDeref(sym *ast.CXArgument, arg *ast.CXArgument) {
	if arg.StructType == nil || !hasDerefOp(sym, constants.DEREF_POINTER) {
		return
	}
	specs := append([]int(nil), arg.DeclarationSpecifiers...)
	for _, deref := range sym.DereferenceOperations {
		if len(specs) == 0 {
			return
		}
		top := specs[len(specs)-1]
		switch {
		case deref == constants.DEREF_POINTER && top == constants.DECL_POINTER:
		case (deref == constants.DEREF_ARRAY || deref == constants.DEREF_SLICE) && (top == constants.DECL_ARRAY || top == constants.DECL_SLICE):
		default:
			return
		}
		specs = specs[:len(specs)-1]
	}
	if len(specs) == 2 && specs[0] == constants.DECL_STRUCT && specs[1] == constants.DECL_POINTER {
		sym.DereferenceOperations = append(sym.DereferenceOperations, constants.DEREF_POINTER)
	}
}

// CheckRedeclared checks if `expr` represents a variable declaration and then checks if an
// instance of that variable has already been declared.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		symbols - a slice of string-CXArg map which corresponds to
//				  the scoping of the  CXArguments. Each element in the slice
//				  corresponds to a different scope. The innermost scope is
//				  the last element of the slice.
//	 expr - the expression.
//	 typeSigIdx - the index of the type signature from the main CXTypeSignature array.
func CheckRedeclared(prgrm *ast.CXProgram, symbolsData *SymbolsData, expr *ast.CXExpression, typeSigIdx ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0810.
	typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0811.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0812.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	if expressionOperator == nil && len(expression.GetOutputs(prgrm)) > 0 && len(expression.GetInputs(prgrm)) == 0 {
		assert.AssertInvariantTrue(len(symbolsData.symbolsIndex) > 0, "CheckRedeclared: symbol scope stack is empty")
		lastIdx := len(symbolsData.symbolsIndex) - 1

		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0813.
		symPkg, err := prgrm.GetPackageFromArray(typeSig.Package)
		if err != nil {
			panic(err)
		}

		_, found := (symbolsData.symbolsIndex)[lastIdx][SymbolsDataKey{SymbolsDataKeyPackage: symPkg.Name, SymbolsDataKeyName: typeSig.Name}]
		switch expr.ExpressionType {
		case ast.CXEXPR_SHORT_DECLARATION_SHARED, ast.CXEXPR_SHORT_DECLARATION_SHARED_LAST:
			// https://go.dev/ref/spec#Short_variable_declarations
			if !symbolsData.shortDeclarationOpen {
				symbolsData.shortDeclarationFirst, symbolsData.shortDeclarationOpen = typeSig.ArgDetails, true
			}
			symbolsData.shortDeclarationNew = symbolsData.shortDeclarationNew || !found
			if expr.ExpressionType == ast.CXEXPR_SHORT_DECLARATION_SHARED_LAST {
				if !symbolsData.shortDeclarationNew {
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeShortDeclarationNoNew, symbolsData.shortDeclarationFirst, ast.CompileErrorOperands{})
				}
				symbolsData.shortDeclarationNew = false
				symbolsData.shortDeclarationOpen = false
			}
		default:
			if found {
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeRedeclared, typeSig.ArgDetails, ast.CompileErrorOperands{int64(typeSig.Name)})
			}
		}
	}
}

// ProcessGoTos sets the ThenLines value if the expression is a goto.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 exprs - slice of expressions.
func ProcessGoTos(prgrm *ast.CXProgram, exprs []ast.CXExpression) {
	for i, expr := range exprs {
		expression, _, err := prgrm.GetOperation(&expr)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0814.
		expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
		expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
		expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

		opGotoFn := ast.Natives[constants.OP_GOTO]
		isOpGoto := false
		if expressionOperator != nil && expressionOperator.AtomicOPCode == opGotoFn.AtomicOPCode && len(expressionOperatorInputs) == len(opGotoFn.Inputs) && len(expressionOperatorOutputs) == len(opGotoFn.Outputs) {
			isOpGoto = true
		}
		if isOpGoto {
			// then it's a goto
			labelFound := false
			for j, e := range exprs {
				expressionToGoTo, _, err := prgrm.GetOperation(&e)
				if err != nil {
					panic(err)
				}

				if expressionToGoTo.Label == expression.Label && i != j {
					// ElseLines is used because arg's default val is false
					expression.ThenLines = j - i - 1
					labelFound = true
					break
				}
			}
			// A return jumps past the end with a generated label and no target.
			if !labelFound && !strings.HasPrefix(prgrm.CXProgramNameText(expression.Label), constants.LABEL_PREFIX) {
				file, line := exprLine(prgrm, exprs, i)
				prgrm.CXProgramCompileErrorAddLine(ast.CompileErrorCodeGotoLabelMissing, file, line, ast.CompileErrorOperands{int64(expression.Label)})
			}
		}
	}
}

// AddExprsToFunction add all expressions to the function.
func AddExprsToFunction(prgrm *ast.CXProgram, fnIdx ast.CXFunctionIndex, exprs []ast.CXExpression) {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0815.
	fn := prgrm.GetFunctionFromArray(fnIdx)
	for _, expr := range exprs {
		fn.AddExpression(prgrm, &expr)
	}
}

// expressionOperatorName is the name of a built-in operator or a function.
func expressionOperatorName(prgrm *ast.CXProgram, operator *ast.CXFunction) string {
	if operator.IsBuiltIn() {
		return operator.BuiltInName()
	}
	return prgrm.CXProgramNameText(operator.Name)
}

func checkMatchParamTypes(prgrm *ast.CXProgram, expr *ast.CXExpression, expected, received []ast.CXTypeSignatureIndex, isInputs bool) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0816.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0817.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	for i, expectedTypeSigIdx := range expected {
		var receivedType, expectedType string

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0818.
		expectedTypeSig := prgrm.GetCXTypeSignatureFromArray(expectedTypeSigIdx)
		// An atomic expected type is neither a pointer argument nor undefined.
		var expectedPointer, expectedUndefined, expectedSlice bool
		if expectedTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0820.
			expectedArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expectedTypeSig.Meta))
			expectedPointer = expectedArg.IsPointer()
			expectedUndefined = expectedArg.Type == types.UNDEFINED
			expectedSlice = expectedUndefined && hasDeclSpec(expectedArg, constants.DECL_SLICE)
		}
		expectedType = ast.GetFormattedType(prgrm, expectedTypeSig)

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0821.
		receivedTypeSig := prgrm.GetCXTypeSignatureFromArray(received[i])
		receivedType = ast.GetFormattedType(prgrm, receivedTypeSig)
		if isInputs && !(expr.IsMethodCall() && i == 0) {
			receivedType = valueFormattedType(prgrm, receivedTypeSig)
		}

		// ProcessMethodCall passes a receiver by reference to a pointer receiver.
		if isInputs && expr.IsMethodCall() && i == 0 && (expectedPointer || expectedTypeSig.Type == ast.TYPE_POINTER_STRUCT) && !strings.HasPrefix(receivedType, "*") {
			expectedType = strings.TrimPrefix(expectedType, "*")
		}

		// A pointer where append and the other []T built-ins take a slice (skycoin-cx#52).
		if isInputs && expectedSlice && strings.HasPrefix(receivedType, "*") {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeSliceArgumentType, receivedTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(expressionOperatorName(prgrm, expressionOperator)), prgrm.CXProgramCompileErrorText(receivedType)})
		}

		if expectedType != receivedType && !expectedUndefined {
			opName := expressionOperatorName(prgrm, expressionOperator)

			if isInputs {
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeFunctionInputType, receivedTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(opName), prgrm.CXProgramCompileErrorText(expectedType), prgrm.CXProgramCompileErrorText(receivedType)})
			} else {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0822.
				expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[i])
				// A call element of a []T literal writes its result to the element.
				if bytes.HasPrefix(prgrm.CXProgramNameBytes(expressionOutputTypeSig.Name), []byte(constants.ELEMENT_PREFIX)) {
					prgrm.CXProgramCompileErrorAddDebug(literalElementTypeCode(prgrm, expressionOutputTypeSig.Name), expressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(expectedType), prgrm.CXProgramCompileErrorText(receivedType)})
					continue
				}
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeFunctionReceiverType, expressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(opName), prgrm.CXProgramCompileErrorText(expectedType), prgrm.CXProgramCompileErrorText(receivedType)})

			}

		}

		var expressionOutputTypeSig *ast.CXTypeSignature
		if expressionOperator.AtomicOPCode == constants.OP_IDENTITY {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0823.
			expressionOutputTypeSig = prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		}
		// In the case of assignment we need to check that the input's type matches the output's type.
		// FIXME: There are some expressions added by the cxgo where temporary variables are used.
		// These temporary variables' types are not properly being set. That's why we use !cxcore.IsTempVar to
		// exclude these cases for now.
		if expressionOperator.AtomicOPCode == constants.OP_IDENTITY && !IsTempVar(prgrm, expressionOutputTypeSig.Name) {
			var outType, inpType string

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0824.
			expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
			inpType = valueFormattedType(prgrm, expressionInputTypeSig)

			var expressionOutputArg *ast.CXArgument
			expressionOutputTypeSigName := expressionOutputTypeSig.Name
			if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0826.
				expressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
				expressionOutputTypeSigName = expressionOutputArg.GetAssignmentElement(prgrm).Name
			}

			outType = assignmentTargetFormattedType(prgrm, expressionOutputTypeSig)

			// We use `isInputs` to only print the error once.
			// Otherwise we'd print the error twice: once for the input and again for the output
			if inpType != outType && isInputs {
				targetName := StripNameNumber(prgrm, expressionOutputTypeSigName)
				if bytes.HasPrefix(prgrm.CXProgramNameBytes(targetName), []byte(constants.ELEMENT_PREFIX)) {
					// The element of a []T literal is at the element start.
					prgrm.CXProgramCompileErrorAddDebug(literalElementTypeCode(prgrm, targetName), expressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(inpType), prgrm.CXProgramCompileErrorText(outType)})
				} else {
					code, operands := assignValueTypeError(prgrm, inpType, targetName, outType)
					prgrm.CXProgramCompileErrorAddDebug(code, expressionInputTypeSig.ArgDetails, operands)
				}
			}

		}
	}
}

// checkMatchExprNumOfInputs records a call with a wrong number of inputs and
// returns false.
func checkMatchExprNumOfInputs(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator, expressionOperatorInputs []ast.CXTypeSignatureIndex, exprCXLine *ast.CXLine, builtIn bool) bool {
	// checking if number of inputs is not the same as the required number of inputs
	if len(expression.GetInputs(prgrm)) == len(expressionOperatorInputs) {
		return true
	}

	// built-ins without declared inputs, such as break and goto, take a label
	if builtIn && len(expressionOperatorInputs) == 0 {
		return true
	}

	// if the last input is of type undefined then it might be a variadic function, such as printf
	if len(expressionOperatorInputs) > 0 && len(expression.GetInputs(prgrm)) > len(expressionOperatorInputs) {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0827.
		lastTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOperatorInputs[len(expressionOperatorInputs)-1])
		switch lastTypeSig.Type {
		case ast.TYPE_CXARGUMENT_DEPRECATE:
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0828.
			if prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastTypeSig.Meta)).Type == types.UNDEFINED {
				return true
			}
		case ast.TYPE_ATOMIC:
			if types.Code(lastTypeSig.Meta) == types.UNDEFINED {
				return true
			}
		}
	}

	// then we need to be strict in the number of inputs
	prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeOperatorInputCount, exprCXLine.FileName, exprCXLine.LineNumber, exprCXLine.LineColumn, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(expression.GetOperatorName(prgrm)), int64(len(expressionOperatorInputs)), int64(len(expression.GetInputs(prgrm)))})
	return false
}

// checkMatchExprNumOfOutputs records a call with a wrong number of receivers
// and returns false.
func checkMatchExprNumOfOutputs(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator, expressionOperatorOutputs []ast.CXTypeSignatureIndex, exprCXLine *ast.CXLine) bool {
	// checking if number of outputs is not the same as the required number of outputs
	if len(expression.GetOutputs(prgrm)) != len(expressionOperatorOutputs) {
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeOperatorOutputCount, exprCXLine.FileName, exprCXLine.LineNumber, exprCXLine.LineColumn, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(expression.GetOperatorName(prgrm)), int64(len(expressionOperatorOutputs)), int64(len(expression.GetOutputs(prgrm)))})
		return false
	}
	return true
}

// CheckTypes checks if the expected types are provided and outputted
// correctly. It returns false after a call with a wrong number of inputs or
// receivers, whose other checks would index past its arguments.
func CheckTypes(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) bool {
	expression, err := prgrm.GetCXAtomicOpFromExpressions(exprs, currIndex)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0829.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	exprCXLine, _ := prgrm.GetPreviousCXLine(exprs, currIndex)

	if expressionOperator != nil {
		expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
		expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

		// A call with the wrong inputs reports that alone, not its receivers too.
		if !checkMatchExprNumOfInputs(prgrm, expression, expressionOperatorInputs, exprCXLine, expressionOperator.IsBuiltIn()) || !checkMatchExprNumOfOutputs(prgrm, expression, expressionOperatorOutputs, exprCXLine) {
			return false
		}
	}

	if expressionOperator != nil && expressionOperator.IsBuiltIn() && expressionOperator.AtomicOPCode == constants.OP_IDENTITY {
		for i := range expression.GetInputs(prgrm) {
			var expectedType string
			var receivedType string

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0830.
			expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[i])
			if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0831.
				expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

				if expressionInputArg.GetAssignmentElement(prgrm).StructType != nil {
					// then it's custom type
					receivedType = prgrm.CXProgramNameText(expressionInputArg.GetAssignmentElement(prgrm).StructType.Name)
				} else {
					// then it's native type
					receivedType = expressionInputArg.GetAssignmentElement(prgrm).Type.Name()

					if expressionInputArg.GetAssignmentElement(prgrm).Type == types.POINTER {
						receivedType = expressionInputArg.GetAssignmentElement(prgrm).PointerTargetType.Name()
					}
				}

			} else {
				receivedType = expressionInputTypeSig.GetTypeString(prgrm)
			}

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0832.
			expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[i])
			var expressionOutputArg *ast.CXArgument
			// baseOutputArg is the struct of a struct literal, expressionOutputArg its field.
			var baseOutputArg *ast.CXArgument
			if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0834.
				expressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
				baseOutputArg = expressionOutputArg
				expressionOutputArg = expressionOutputArg.GetAssignmentElement(prgrm)

				if expressionOutputArg.StructType != nil {
					// then it's custom type
					expectedType = prgrm.CXProgramNameText(expressionOutputArg.StructType.Name)
				} else {
					// then it's native type
					expectedType = expressionOutputArg.Type.Name()

					if expressionOutputArg.Type == types.POINTER {
						expectedType = expressionOutputArg.PointerTargetType.Name()
					}
				}
			} else {
				expectedType = expressionOutputTypeSig.GetTypeString(prgrm)
			}

			if receivedType != expectedType {
				if exprs[currIndex].IsStructLiteral() && baseOutputArg != nil && baseOutputArg.StructType != nil && len(baseOutputArg.Fields) > 0 {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0835.
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeStructLiteralFieldType, expressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{int64(prgrm.GetCXArgFromArray(baseOutputArg.Fields[len(baseOutputArg.Fields)-1]).Name), int64(baseOutputArg.StructType.Name), prgrm.CXProgramCompileErrorText(expectedType), prgrm.CXProgramCompileErrorText(receivedType)})
				} else if IsTempVar(prgrm, expressionOutputTypeSig.Name) || ShortCircuitJumpCheck(prgrm, exprs, currIndex) {
					// Otherwise checkMatchParamTypes reports the full types below.
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeAssignSymbolType, expressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(receivedType), int64(expressionOutputTypeSig.Name), prgrm.CXProgramCompileErrorText(expectedType)})
				}
			}

		}
	}

	// then it's a function call and not a declaration
	if expressionOperator != nil && !ShortCircuitJumpCheck(prgrm, exprs, currIndex) {
		// checking inputs matching operator's inputs
		checkMatchParamTypes(prgrm, &exprs[currIndex], expressionOperator.GetInputs(prgrm), expression.GetInputs(prgrm), true)

		// checking outputs matching operator's outputs
		checkMatchParamTypes(prgrm, &exprs[currIndex], expressionOperator.GetOutputs(prgrm), expression.GetOutputs(prgrm), false)
	}
	return true
}

// ProcessStringAssignment sets the args PassBy to PASSBY_VALUE if the type is string.
func ProcessStringAssignment(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0836.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0837.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

	opIdentFn := ast.Natives[constants.OP_IDENTITY]
	isOpIdent := false
	if expressionOperator != nil && expressionOperator.AtomicOPCode == opIdentFn.AtomicOPCode && len(expressionOperatorInputs) == len(opIdentFn.Inputs) && len(expressionOperatorOutputs) == len(opIdentFn.Outputs) {
		isOpIdent = true
	}
	if isOpIdent {
		for i, outputIdx := range expression.GetOutputs(prgrm) {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0838.
			output := prgrm.GetCXTypeSignatureFromArray(outputIdx)

			// Only a CXArgument can hold a str or aff value, typed signatures are never strings.
			if output.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
				continue
			}
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0839.
			out := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(output.Meta))

			if len(expression.GetInputs(prgrm)) > i {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0840.
				expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[i])
				if expressionInputTypeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
					continue
				}
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0841.
				expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

				out = out.GetAssignmentElement(prgrm)
				inp := expressionInputArg.GetAssignmentElement(prgrm)

				if (out.Type == types.STR || out.Type == types.AFF) && out.Name != stringpool.StringGidNil &&
					(inp.Type == types.STR || inp.Type == types.AFF) && inp.Name != stringpool.StringGidNil {
					out.PassBy = constants.PASSBY_VALUE
				}
			}
		}
	}
}

// ProcessReferenceAssignment checks if the reference of a symbol can be assigned to the expression's output.
// For example: `var foo i32; var bar i32; bar = &foo` is not valid.
// func ProcessReferenceAssignment(prgrm *ast.CXProgram, expr *ast.CXExpression) {
// 	expression, err := prgrm.GetCXAtomicOp(expr.Index)
// 	if err != nil {
// 		panic(err)
// 	}

// 	for _, outIdx := range expression.Outputs {
// 		out := prgrm.GetCXArgFromArray(outIdx)
// 		elt := out.GetAssignmentElement(prgrm)
// 		if elt.PassBy == constants.PASSBY_REFERENCE &&
// 			!hasDeclSpec(elt, constants.DECL_POINTER) &&
// 			elt.PointerTargetType != types.STR && elt.Type != types.STR && !elt.IsSlice {
// 			println(ast.CompilationError(prgrm.CXProgramSourceFileText(CurrentFile), LineNo), "invalid reference assignment", elt.Name)
// 		}
// 	}

// }

// ProcessShortDeclaration sets proper values if the expr is a short declaration.
func ProcessShortDeclaration(prgrm *ast.CXProgram, expr *ast.CXExpression, expressions []ast.CXExpression, idx int, offset *types.Pointer) {
	if len(expressions) <= 1 {
		return
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0842.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	var expressionOutputArgType types.Code
	if len(expression.GetOutputs(prgrm)) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0843.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0844.
			expressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
			expressionOutputArgType = expressionOutputArg.Type
		} else {
			expressionOutputArgType = expressionOutputTypeSig.GetType(prgrm)
		}
	}

	// process short declaration
	if len(expression.GetOutputs(prgrm)) > 0 && len(expression.GetInputs(prgrm)) > 0 && expressionOutputArgType == types.IDENTIFIER && !expr.IsStructLiteral() && !isParseOp(prgrm, expr) {
		prevExpression, err := prgrm.GetPreviousCXAtomicOpFromExpressions(expressions, idx-1)
		// An undeclared target has no declaration before it, and was reported already.
		if err != nil || len(prevExpression.GetOutputs(prgrm)) == 0 {
			return
		}

		var argType, argPointerTargetType types.Code
		var argSize types.Pointer
		var argStructType *ast.CXStruct
		if expr.IsMethodCall() {

			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now an arena getter, site S0845.
			expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
			expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)
			if len(expressionOperatorOutputs) == 0 {
				return
			}
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0846.
			expressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOperatorOutputs[0])

			if expressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0847.
				expressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOperatorOutputTypeSig.Meta))
				argType = expressionOperatorOutputArg.Type
				argPointerTargetType = expressionOperatorOutputArg.PointerTargetType
				argSize = expressionOperatorOutputArg.Size
			} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
				argType = types.Code(expressionOperatorOutputTypeSig.Meta)
				argSize = types.Code(expressionOperatorOutputTypeSig.Meta).Size()
			} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				argType = types.Code(expressionOperatorOutputTypeSig.Meta)
				argSize = types.Code(expressionOperatorOutputTypeSig.Meta).Size()
			} else {
				panic("type is not known")
			}
		} else {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0848.
			expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
			if t, ok := shortDeclarationPointerType(prgrm, expressionInputTypeSig); ok && shortDeclarationCollectionSet(prgrm, prevExpression, expression, t, offset) {
				return
			}
			if t, ok := shortDeclarationCollectionType(prgrm, expressionInputTypeSig); ok && shortDeclarationCollectionSet(prgrm, prevExpression, expression, t, offset) {
				return
			}
			if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0849.
				expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
				argType = expressionInputArg.Type
				argPointerTargetType = expressionInputArg.PointerTargetType
				argSize = expressionInputArg.Size
				// A variable declared from a struct literal has the type of a field.
				if expressionInputArg.StructType != nil && len(expressionInputArg.Fields) == 0 && len(expressionInputArg.Lengths) == 0 && expressionInputArg.Type != types.POINTER {
					argType, argSize, argStructType = types.STRUCT, expressionInputArg.StructType.GetStructSize(prgrm), expressionInputArg.StructType
				}
			} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
				argType = types.Code(expressionInputTypeSig.Meta)
				argSize = types.Code(expressionInputTypeSig.Meta).Size()
			} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				argType = types.Code(expressionInputTypeSig.Meta)
				argSize = types.Code(expressionInputTypeSig.Meta).Size()
			} else if expressionInputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S0850.
				arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
				argType = types.Code(arrDetails.Meta)
				argSize = types.Code(arrDetails.Meta).Size()
			} else if expressionInputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S0851.
				sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
				argType = types.Code(sliceDetails.Meta)
				argSize = types.Code(sliceDetails.Meta).Size()
			} else if expressionInputTypeSig.Type == ast.TYPE_STRUCT || expressionInputTypeSig.Type == ast.TYPE_POINTER_STRUCT {
				argType, argPointerTargetType, argSize, argStructType = shortDeclarationStructValue(prgrm, expressionInputTypeSig)
			} else {
				panic("type is not known")
			}
		}

		// x := &v declares a pointer to the type of v.
		reference := !expr.IsMethodCall() && shortDeclarationReference(prgrm, prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0]), argType)

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0852.
		prevExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prevExpression.GetOutputs(prgrm)[0])
		var relocatedOffset types.Pointer
		relocated := false
		var prevExpressionOutputIdx ast.CXArgumentIndex
		if prevExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			prevExpressionOutputIdx = ast.CXArgumentIndex(prevExpressionOutputTypeSig.Meta)
			reservedSize := ast.GetArgSize(prgrm, prgrm.GetCXArg(prevExpressionOutputIdx))

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1682.
			prgrm.GetCXArg(ast.CXArgumentIndex(prevExpressionOutputIdx)).Type = argType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1683.
			prgrm.GetCXArg(ast.CXArgumentIndex(prevExpressionOutputIdx)).PointerTargetType = argPointerTargetType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1684.
			prgrm.GetCXArg(ast.CXArgumentIndex(prevExpressionOutputIdx)).Size = argSize
			shortDeclarationStructTypeSet(prgrm.GetCXArg(prevExpressionOutputIdx), argType, argStructType)
			if reference {
				shortDeclarationPointerSet(prgrm.GetCXArg(prevExpressionOutputIdx))
			}
			// A value larger than the frame slot reserved for the declaration moves it.
			if declSize := ast.GetArgSize(prgrm, prgrm.GetCXArg(prevExpressionOutputIdx)); declSize > reservedSize {
				relocated, relocatedOffset = true, *offset
				prgrm.GetCXArg(prevExpressionOutputIdx).Offset = relocatedOffset
				prevExpressionOutputTypeSig.Offset = relocatedOffset
				updateSymbolsTableOffsetAdd(prgrm, prevExpressionOutputTypeSig, offset, declSize)
			}
		} else if prevExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			prevExpressionOutputTypeSig.Meta = int(argType)
		} else if prevExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			prevExpressionOutputTypeSig.Meta = int(argType)
		} else if prevExpressionOutputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0853.
			arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(prevExpressionOutputTypeSig.Meta)
			arrDetails.Meta = int(argType)

		} else if prevExpressionOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0854.
			sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(prevExpressionOutputTypeSig.Meta)
			sliceDetails.Meta = int(argType)

		} else {
			panic("type is not known")
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0855.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			expressionOutputIdx := ast.CXArgumentIndex(expressionOutputTypeSig.Meta)

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1685.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Type = argType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1686.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).PointerTargetType = argPointerTargetType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1687.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Size = argSize
			shortDeclarationStructTypeSet(prgrm.GetCXArg(expressionOutputIdx), argType, argStructType)
			if reference {
				shortDeclarationPointerSet(prgrm.GetCXArg(expressionOutputIdx))
			}
			if relocated {
				prgrm.GetCXArg(expressionOutputIdx).Offset = relocatedOffset
				expressionOutputTypeSig.Offset = relocatedOffset
			}
		} else if expressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			expressionOutputTypeSig.Meta = int(argType)
		} else if expressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			expressionOutputTypeSig.Meta = int(argType)
		} else if expressionOutputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0856.
			arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionOutputTypeSig.Meta)
			arrDetails.Meta = int(argType)
		} else if expressionOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0857.
			sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionOutputTypeSig.Meta)
			sliceDetails.Meta = int(argType)
		} else {
			panic("type is not known")
		}
	}
}

// ProcessSlice sets DereferenceOperations if the arg is a slice.
func ProcessSlice(prgrm *ast.CXProgram, inpIdx ast.CXArgumentIndex) {
	if inpIdx == -1 {
		return
	}

	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S0858.
	inp := prgrm.GetCXArgFromArray(inpIdx)

	var elt *ast.CXArgument
	if len(inp.Fields) > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0860.
		elt = prgrm.GetCXArgFromArray(inp.Fields[len(inp.Fields)-1])
	} else {
		elt = inp
	}

	// *ps[i] on a []*T dereferences the element, which is not a slice.
	if elt.IsSlice() && len(elt.Indexes) < len(elt.Lengths) && len(elt.DereferenceOperations) > 0 && elt.DereferenceOperations[len(elt.DereferenceOperations)-1] == constants.DEREF_POINTER {
		elt.DereferenceOperations = elt.DereferenceOperations[:len(elt.DereferenceOperations)-1]
		return
	}

	if elt.IsSlice() && len(elt.DereferenceOperations) > 0 && len(inp.Fields) == 0 {
		return
		// elt.DereferenceOperations = append([]int{cxcore.DEREF_POINTER}, elt.DereferenceOperations...)
	}
}

// ProcessSliceAssignent sets correct values for slice assignment expressions.
func ProcessSliceAssignment(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0861.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0862.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

	opIdentFn := ast.Natives[constants.OP_IDENTITY]
	isOpIdent := false
	if expressionOperator != nil && expressionOperator.AtomicOPCode == opIdentFn.AtomicOPCode && len(expressionOperatorInputs) == len(opIdentFn.Inputs) && len(expressionOperatorOutputs) == len(opIdentFn.Outputs) {
		isOpIdent = true
	}

	if isOpIdent {
		var inp *ast.CXArgument
		var out *ast.CXArgument

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0865.
		expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
		var expressionInputArg *ast.CXArgument
		if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0867.
			expressionInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
		} else {
			// panic("type is not cx argument deprecate\n\n")
			return
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0868.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		var expressionOutputArg *ast.CXArgument
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0870.
			expressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
		} else {
			// panic("type is not cx argument deprecate\n\n")
			return
		}

		inp = expressionInputArg.GetAssignmentElement(prgrm)
		out = expressionOutputArg.GetAssignmentElement(prgrm)

		if inp.IsSlice() && out.IsSlice() && len(inp.Indexes) == 0 && len(out.Indexes) == 0 {
			out.PassBy = constants.PASSBY_VALUE
		}
	}
	if expressionOperator != nil && !expressionOperator.IsBuiltIn() {
		// then it's a function call
		for _, inputIdx := range expression.GetInputs(prgrm) {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0871.
			input := prgrm.GetCXTypeSignatureFromArray(inputIdx)

			// Only a CXArgument slice needs its PassBy changed here.
			if input.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
				continue
			}
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0872.
			inp := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(input.Meta))

			assignElt := inp.GetAssignmentElement(prgrm)

			// we want to pass by value if we're sending the slice as a whole (no indexing)
			// unless it's a pointer to the slice
			if assignElt.IsSlice() && len(assignElt.Indexes) == 0 && !hasDeclSpec(assignElt, constants.DECL_POINTER) {
				assignElt.PassBy = constants.PASSBY_VALUE
			}
		}
	}
}

// lookupSymbol searches for `ident` in `symbols`, starting from the innermost scope.
func lookupSymbol(prgrm *ast.CXProgram, pkgName stringpool.StringGid, ident stringpool.StringGid, symbolsData *SymbolsData) (*ast.CXTypeSignature, error) {
	fullName := SymbolsDataKey{SymbolsDataKeyPackage: pkgName, SymbolsDataKeyName: StripNameNumber(prgrm, ident)}
	for c := len(symbolsData.symbolsIndex) - 1; c >= 0; c-- {
		if symIdx, found := (symbolsData.symbolsIndex)[c][fullName]; found {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1776.
			return prgrm.GetCXTypeSignatureFromArray(symbolsData.symbols[symIdx]), nil
		}
	}

	// Checking if `ident` refers to a function.
	pkg, err := prgrm.GetPackage(pkgName)
	if err != nil {
		return nil, err
	}

	notFound := errors.New("identifier '" + prgrm.CXProgramNameText(ident) + "' does not exist")
	// We're not checking for that error
	fn, err := pkg.GetFunction(prgrm, ident)
	if err != nil {
		return nil, errors.New(err.Error() + ":" + notFound.Error() + fmt.Sprintf("--fullName=%s.%s", prgrm.CXProgramNameText(pkgName), prgrm.CXProgramNameText(fullName.SymbolsDataKeyName)))
	}
	// Then we found a function by that name. Let's create a `cxcore.CXArgument` of
	// type `func` with that name.
	_, fnArg := ast.MakeArgument(prgrm, ident, fn.FileName, fn.FileLine)
	fnArg.SetType(types.FUNC)
	fnArg.Package = ast.CXPackageIndex(pkg.Index)

	fnArgIdx := ast.CXArgumentIndex(fnArg.Index)
	_, fnArgTypeSig := prgrm.AllocCXTypeSignature()
	fnArgTypeSig.Name = fnArg.Name
	fnArgTypeSig.Offset = fnArg.Offset
	fnArgTypeSig.Package = fnArg.Package
	fnArgTypeSig.Type = ast.TYPE_CXARGUMENT_DEPRECATE
	fnArgTypeSig.Meta = int(fnArgIdx)
	fnArgTypeSig.ArgDetails = fnArg.ArgDetails
	symbolsData.functionSymbols = append(symbolsData.functionSymbols, fnArgTypeSig.Index)

	return fnArgTypeSig, nil
}

// UpdateSymbolsTable adds `sym` to the innermost scope (last element of slice) in `symbols`.
func UpdateSymbolsTable(prgrm *ast.CXProgram, symbolsData *SymbolsData, typeSigIdx ast.CXTypeSignatureIndex, offset *types.Pointer, shouldAlreadyExist bool) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0876.
	typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)

	var sym *ast.CXArgument
	if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0878.
		sym = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))
		if sym.Type == types.UNDEFINED && !IsTempVar(prgrm, typeSig.Name) {
			return
		}
	} else {
		// panic("type is not cx arg deprecate\n\n")
		// TODO: temporary return only
		sym = nil
	}

	if typeSig.Name != stringpool.StringGidNil {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0879.
		typeSigPkg, err := prgrm.GetPackageFromArray(typeSig.Package)
		if err != nil {
			panic(err)
		}

		currFn, err := prgrm.GetCurrentFunction()
		if err != nil {
			panic("error getting current function")
		}

		if !currFn.IsLocalVariable(typeSig.Name) {
			GetGlobalSymbol(prgrm, symbolsData, typeSigPkg, typeSig.Name, typeSig.ArgDetails)
		}

		assert.AssertInvariantTrue(len(symbolsData.symbolsIndex) > 0, "UpdateSymbolsTable: symbol scope stack is empty")
		lastIdx := len(symbolsData.symbolsIndex) - 1
		fullName := SymbolsDataKey{SymbolsDataKeyPackage: typeSigPkg.Name, SymbolsDataKeyName: StripNameNumber(prgrm, typeSig.Name)}

		// outerSym, err := lookupSymbol(sym.Package.Name, sym.Name, symbols)
		_, err = lookupSymbol(prgrm, typeSigPkg.Name, typeSig.Name, symbolsData)
		_, found := (symbolsData.symbolsIndex)[lastIdx][fullName]

		// then it wasn't found in any scope
		if err != nil && shouldAlreadyExist {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeIdentifierMissing, typeSig.ArgDetails, ast.CompileErrorOperands{int64(typeSig.Name)})
		}

		// then it was already added in the innermost scope
		if found {
			return
		}

		// then it is a new declaration
		if !shouldAlreadyExist && !found {
			// We remove it from local var array since it is already added to the symbols
			err := currFn.RemoveLocalVariableFromArray(typeSig.Name)
			if err != nil {
				panic(err)
			}

			if !IsTempVar(prgrm, typeSig.Name) {
				scopeShadowCheck(prgrm, symbolsData, typeSigPkg, typeSig)
				// Change name to n:varname format
				typeSig.Name = prgrm.CXProgramNameIntern(fmt.Sprintf("%v:%s", symbolsData.varCount, prgrm.CXProgramNameText(typeSig.Name)))
				symbolsData.varCount++
			}

			symbolsData.symbols = append(symbolsData.symbols, typeSigIdx)
			// add name to symbols
			symbolsData.symbolsIndex[lastIdx][fullName] = len(symbolsData.symbols) - 1

			if symbolsData.escaped[fullName.SymbolsDataKeyName] {
				escapeDeclare(prgrm, symbolsData, typeSig, sym, offset)
			} else if sym != nil {
				sym.Name = typeSig.Name
				sym.Offset = *offset
				updateSymbolsTableOffsetAdd(prgrm, typeSig, offset, ast.GetArgSize(prgrm, sym))
			} else {
				typeSig.Offset = *offset
				updateSymbolsTableOffsetAdd(prgrm, typeSig, offset, typeSig.GetSize(prgrm, true))
			}
			gcRootsDeclare(prgrm, currFn, typeSig, sym)
		}
	}
}

// updateSymbolsTableOffsetAdd moves the frame offset past size bytes of
// typeSig, recording a frame that does not fit declarationSizeLimit.
func updateSymbolsTableOffsetAdd(prgrm *ast.CXProgram, typeSig *ast.CXTypeSignature, offset *types.Pointer, size types.Pointer) {
	if !declarationSizeSumFits(*offset, size) {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeDeclarationTooLarge, typeSig.ArgDetails, ast.CompileErrorOperands{int64(StripNameNumber(prgrm, typeSig.Name))})
	}
	*offset += size
}

// ProcessMethodCall resolves the method of a method call expression. The
// receiver is the first input and names the method as its last field. It
// returns false after recording why the method cannot be resolved.
func ProcessMethodCall(prgrm *ast.CXProgram, expr *ast.CXExpression, symbolsData *SymbolsData) bool {
	if !expr.IsMethodCall() {
		return true
	}
	expression := &prgrm.CXAtomicOps[expr.Index]
	receiverTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
	assert.AssertInvariantTrue(receiverTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE, "ProcessMethodCall: receiver is not an argument")
	receiver := prgrm.GetCXArg(ast.CXArgumentIndex(receiverTypeSig.Meta))
	receiverPkg, err := prgrm.GetPackageFromArray(receiver.Package)
	if err != nil {
		panic(err)
	}

	// The receiver may be a global, as box of box.Area().
	GetGlobalSymbol(prgrm, symbolsData, receiverPkg, receiver.Name, receiver.ArgDetails)
	symbolTypeSig, err := lookupSymbol(prgrm, receiverPkg.Name, receiver.Name, symbolsData)
	if err != nil {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeIdentifierMissing, receiver.ArgDetails, ast.CompileErrorOperands{int64(receiver.Name)})
		methodCallInvalidate(prgrm, expression)
		return false
	}

	strct := symbolTypeSig.GetStructType(prgrm)
	typeSig := symbolTypeSig
	receiverPointer := typeSignaturePointer(prgrm, symbolTypeSig)
	methodIdx := receiver.Fields[len(receiver.Fields)-1]
	// indirect is a receiver reached through a pointer, which is addressable.
	indirect := false
	for _, fieldIdx := range receiver.Fields[:len(receiver.Fields)-1] {
		if strct == nil {
			break
		}
		indirect = indirect || receiverPointer
		field, err := strct.GetField(prgrm, prgrm.GetCXArgFromArray(fieldIdx).Name)
		if err != nil {
			break
		}
		typeSig = field
		strct = field.GetStructType(prgrm)
		receiverPointer = typeSignaturePointer(prgrm, field)
	}
	if strct == nil && typeSignatureInvalid(prgrm, typeSig) {
		// A value of no valid type has no methods, and its cause is the error.
		methodCallInvalidate(prgrm, expression)
		return false
	}
	if strct == nil {
		typeText := ast.GetFormattedType(prgrm, typeSig)
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodePrimitiveFieldAccess, receiver.ArgDetails, ast.CompileErrorOperands{int64(StripNameNumber(prgrm, symbolTypeSig.Name)), prgrm.CXProgramCompileErrorText(typeText)})
		methodCallInvalidate(prgrm, expression)
		return false
	}

	strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
	if err != nil {
		panic(err)
	}
	method := prgrm.GetCXArgFromArray(methodIdx)
	fnIdx, err := strctPkg.GetMethod(prgrm, strct.Name, method.Name)
	if err != nil {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeMethodMissing, method.ArgDetails, ast.CompileErrorOperands{int64(strct.Name), int64(method.Name)})
		methodCallInvalidate(prgrm, expression)
		return false
	}
	expression = &prgrm.CXAtomicOps[expr.Index]
	expression.Operator = fnIdx
	receiver.Fields = receiver.Fields[:len(receiver.Fields)-1]
	methodInputs := prgrm.GetFunctionFromArray(fnIdx).GetInputs(prgrm)
	methodPointer := len(methodInputs) > 0 && typeSignaturePointer(prgrm, prgrm.GetCXTypeSignatureFromArray(methodInputs[0]))
	if !receiverPointer && methodPointer {
		if !indirect && receiver.PassBy != constants.PASSBY_REFERENCE && len(receiver.Indexes) == 0 && IsTempVar(prgrm, receiver.Name) {
			// A literal or a call result is not addressable
			// (https://go.dev/ref/spec#Calls).
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeMethodPointerValue, method.ArgDetails, ast.CompileErrorOperands{int64(method.Name), int64(strct.Name)})
		}
		receiver.PassBy = constants.PASSBY_REFERENCE
		receiverTypeSig.PassBy = constants.PASSBY_REFERENCE
	}
	if len(methodInputs) > 0 && !methodPointer {
		methodCallReceiverValue(prgrm, receiver, receiverTypeSig, receiverPointer)
	}

	return methodCallResultsType(prgrm, expression, strct, method)
}

// methodCallReceiverValue passes a pointer receiver to a value receiver as
// the value it points to, as Go does for p.m() and (&t).m().
func methodCallReceiverValue(prgrm *ast.CXProgram, receiver *ast.CXArgument, receiverTypeSig *ast.CXTypeSignature, receiverPointer bool) {
	elt := receiver.GetAssignmentElement(prgrm)
	specs := elt.DeclarationSpecifiers
	if receiver.PassBy == constants.PASSBY_REFERENCE && len(specs) > 0 && specs[len(specs)-1] == constants.DECL_POINTER {
		receiver.PassBy = constants.PASSBY_VALUE
		receiverTypeSig.PassBy = constants.PASSBY_VALUE
		elt.DeclarationSpecifiers = specs[:len(specs)-1]
		return
	}
	if receiverPointer {
		elt.DereferenceOperations = append(elt.DereferenceOperations, constants.DEREF_POINTER)
		elt.DeclarationSpecifiers = append(elt.DeclarationSpecifiers, constants.DECL_DEREF)
	}
}

// methodCallInvalidate types the result variables of a method call that
// cannot be resolved as types.INVALID, so their uses report nothing more.
func methodCallInvalidate(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator) {
	for _, resultIdx := range expression.GetOutputs(prgrm) {
		resultTypeSig := prgrm.GetCXTypeSignatureFromArray(resultIdx)
		if resultTypeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE || !strings.HasPrefix(prgrm.CXProgramNameText(resultTypeSig.Name), constants.METHOD_RESULT_PREFIX) {
			continue
		}
		result := prgrm.GetCXArg(ast.CXArgumentIndex(resultTypeSig.Meta))
		if result.Type == types.UNDEFINED {
			result.Type, result.Size = types.INVALID, 0
		}
	}
}

// typeSignaturePointer reports whether typeSig is a pointer.
func typeSignaturePointer(prgrm *ast.CXProgram, typeSig *ast.CXTypeSignature) bool {
	switch typeSig.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		return prgrm.GetCXArg(ast.CXArgumentIndex(typeSig.Meta)).IsPointer()
	case ast.TYPE_POINTER_STRUCT, ast.TYPE_POINTER_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		return true
	}
	return false
}

// methodCallResultsType gives the result variables of MethodCallValue the
// types of the outputs of the method. It returns false after recording a
// method without the outputs its call uses.
func methodCallResultsType(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator, strct *ast.CXStruct, method *ast.CXArgument) bool {
	methodOutputs := prgrm.GetFunctionFromArray(expression.Operator).GetOutputs(prgrm)
	for i, resultIdx := range expression.GetOutputs(prgrm) {
		resultTypeSig := prgrm.GetCXTypeSignatureFromArray(resultIdx)
		if resultTypeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE || !strings.HasPrefix(prgrm.CXProgramNameText(resultTypeSig.Name), constants.METHOD_RESULT_PREFIX) {
			continue
		}
		result := prgrm.GetCXArg(ast.CXArgumentIndex(resultTypeSig.Meta))
		if result.Type != types.UNDEFINED {
			continue
		}
		if i >= len(methodOutputs) {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeMethodResultMissing, method.ArgDetails, ast.CompileErrorOperands{int64(strct.Name), int64(method.Name)})
			methodCallInvalidate(prgrm, expression)
			return false
		}

		outputTypeSig := prgrm.GetCXTypeSignatureFromArray(methodOutputs[i])
		if outputTypeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
			name, pkg, details := resultTypeSig.Name, resultTypeSig.Package, resultTypeSig.ArgDetails
			index := resultTypeSig.Index
			*resultTypeSig = *outputTypeSig
			resultTypeSig.Index, resultTypeSig.Name, resultTypeSig.Package, resultTypeSig.ArgDetails = index, name, pkg, details
			continue
		}

		output := prgrm.GetCXArg(ast.CXArgumentIndex(outputTypeSig.Meta))
		result.SetType(output.Type)
		result.DeclarationSpecifiers = output.DeclarationSpecifiers
		result.Lengths = output.Lengths
		result.StructType = output.StructType
		result.Size = output.Size
		if output.StructType != nil {
			result.Size = output.StructType.GetStructSize(prgrm)
		}
		result.PointerTargetType = output.PointerTargetType
		result.PassBy = output.PassBy
	}
	return true
}

// GiveOffset
func GiveOffset(prgrm *ast.CXProgram, symbolsData *SymbolsData, typeSigIdx ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0925.
	symTypeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)
	symTypeSigName := StripNameNumber(prgrm, symTypeSig.Name)
	if symTypeSigName != stringpool.StringGidNil {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0926.
		symPkg, err := prgrm.GetPackageFromArray(symTypeSig.Package)
		if err != nil {
			panic(err)
		}

		currFn, err := prgrm.GetCurrentFunction()
		if err != nil {
			// TODO: improve error handling
			panic("error getting current function")
		}

		if !currFn.IsLocalVariable(symTypeSigName) {
			GetGlobalSymbol(prgrm, symbolsData, symPkg, symTypeSigName, symTypeSig.ArgDetails)
		}

		argTypeSignature, err := lookupSymbol(prgrm, symPkg.Name, symTypeSigName, symbolsData)
		if err == nil && dereferenceNonPointer(prgrm, symTypeSig, argTypeSignature) {
			return
		}
		if err == nil {
			ProcessSymbolFields(prgrm, symTypeSig, argTypeSignature)
			CopyArgFields(prgrm, symTypeSig, argTypeSignature)
		}
	}
}

// ProcessTempVariable completes the temp variable expression.
func ProcessTempVariable(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0927.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0928.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

	opIdentFn := ast.Natives[constants.OP_IDENTITY]
	isOpIdent := false
	if expressionOperator != nil && expressionOperator.AtomicOPCode == opIdentFn.AtomicOPCode && len(expressionOperatorInputs) == len(opIdentFn.Inputs) && len(expressionOperatorOutputs) == len(opIdentFn.Outputs) {
		isOpIdent = true
	}

	if expressionOperator != nil && (isOpIdent || ast.IsArithmeticOperator(expressionOperator.AtomicOPCode)) && len(expression.GetOutputs(prgrm)) > 0 && len(expression.GetInputs(prgrm)) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0929.
		outputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		// An element of a literal of pointers or mixed dimensions, tmp[i], has
		// its type from the literal.
		if IsTempVar(prgrm, outputTypeSig.Name) && !argumentIndexedElement(prgrm, outputTypeSig) {
			if outputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				outputArgIdx := ast.CXArgumentIndex(outputTypeSig.Meta)

				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0930.
				expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
				if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0931.
					expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

					// x.f is sized by its field, not by the element of x.
					expressionInput := expressionInputArg.GetAssignmentElement(prgrm)
					// then it's a temporary variable and it needs to adopt its input's type
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1728.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInput.Type
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1729.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).PointerTargetType = expressionInput.PointerTargetType
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1730.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = expressionInput.Size
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1731.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).PreviouslyDeclared = true
				} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1732.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInputTypeSig.GetType(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1733.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = types.Code(expressionInputTypeSig.Meta).Size()
				} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1734.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInputTypeSig.GetType(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1735.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = types.Code(expressionInputTypeSig.Meta).Size()
				} else if expressionInputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC || expressionInputTypeSig.Type == ast.TYPE_SLICE_ATOMIC || expressionInputTypeSig.Type == ast.TYPE_POINTER_ARRAY_ATOMIC || expressionInputTypeSig.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0932.
					arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1736.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInputTypeSig.GetType(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1737.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = types.Code(arrDetails.Meta).Size()
				} else if expressionInputTypeSig.Type == ast.TYPE_STRUCT || expressionInputTypeSig.Type == ast.TYPE_POINTER_STRUCT {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1738.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInputTypeSig.GetType(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1739.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = expressionInputTypeSig.GetSize(prgrm, false)
				} else {
					panic("type is not known")
				}

			} else if outputTypeSig.Type == ast.TYPE_ATOMIC || outputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0933.
				expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
				if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0934.
					expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
					outputTypeSig.Meta = int(expressionInputArg.Type)
				} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
					outputTypeSig.Meta = expressionInputTypeSig.Meta
				} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					outputTypeSig.Meta = expressionInputTypeSig.Meta
				} else if expressionInputTypeSig.Type == ast.TYPE_SLICE_ATOMIC || expressionInputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC || expressionInputTypeSig.Type == ast.TYPE_POINTER_ARRAY_ATOMIC || expressionInputTypeSig.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0935.
					sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
					outputTypeSig.Meta = sliceDetails.Meta
				} else {
					panic("type is not known")
				}
			} else if outputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S0936.
				outputArrDetails := prgrm.GetCXTypeSignatureArrayFromArray(outputTypeSig.Meta)
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0937.
				expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
				if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0938.
					expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
					outputArrDetails.Meta = int(expressionInputArg.Type)
				} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
					outputArrDetails.Meta = expressionInputTypeSig.Meta
				} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					outputArrDetails.Meta = expressionInputTypeSig.Meta
				} else if expressionInputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0939.
					arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
					outputArrDetails.Meta = arrDetails.Meta
				} else {
					panic("type is not known")
				}
			} else if outputTypeSig.Type != ast.TYPE_SLICE_ATOMIC {
				// The value of a slice literal is typed already.
				panic("type is not known")
			}
		}
	}
}

// CopyArgFields copies 'arg' fields to 'sym' fields.
// copyArgFieldsIndexesCheck records indexes past the dimensions of the type
// and drops them, so later passes never index past its lengths.
func copyArgFieldsIndexesCheck(prgrm *ast.CXProgram, sym *ast.CXArgument, typeSig *ast.CXTypeSignature) {
	dimensions := 0
	switch typeSig.Type {
	case ast.TYPE_ATOMIC:
	case ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		dimensions = len(prgrm.GetCXTypeSignatureArrayFromArray(typeSig.Meta).Lengths)
	default:
		return
	}
	if len(sym.Indexes) <= dimensions {
		return
	}
	prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
	sym.Indexes = sym.Indexes[:dimensions]
}

func CopyArgFields(prgrm *ast.CXProgram, symTypeSignature, argTypeSignature *ast.CXTypeSignature) {
	var sym *ast.CXArgument
	if symTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0941.
		sym = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(symTypeSignature.Meta))
	} else {
		sym = nil
	}

	var arg *ast.CXArgument
	if argTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0943.
		arg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argTypeSignature.Meta))
	} else {
		arg = nil
	}

	if sym != nil && arg == nil {
		copyArgFieldsIndexesCheck(prgrm, sym, argTypeSignature)
	}

	// TODO: check if this needs a change
	if sym == nil && arg == nil {
		if !(symTypeSignature.Type == ast.TYPE_SLICE_ATOMIC && argTypeSignature.Type == ast.TYPE_SLICE_ATOMIC) && !(symTypeSignature.Type == ast.TYPE_STRUCT && argTypeSignature.Type == ast.TYPE_STRUCT) && !(symTypeSignature.Type == ast.TYPE_POINTER_STRUCT && argTypeSignature.Type == ast.TYPE_POINTER_STRUCT) {
			// Only copy meta if sym and arg are not type slice atomic.
			symTypeSignature.Meta = argTypeSignature.Meta
		}

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Offset = argTypeSignature.Offset

		return
	} else if sym != nil && ast.IsTypePointerAtomic(sym) && argTypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = argTypeSignature.Meta
		symTypeSignature.Offset = argTypeSignature.Offset

		return

	} else if sym != nil && arg == nil && argTypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
		sym.Name = argTypeSignature.Name
		sym.Package = argTypeSignature.Package
		sym.Type = types.Code(argTypeSignature.Meta)

		sym.Offset = argTypeSignature.Offset

		declSpec := []int{constants.DECL_BASIC, constants.DECL_POINTER}
		for _, spec := range sym.DeclarationSpecifiers {
			// checking if we need to remove or add cxcore.DECL_POINTERs
			// also we could be removing
			switch spec {
			case constants.DECL_INDEXING:
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
			case constants.DECL_DEREF:
				if declSpec[len(declSpec)-1] == constants.DECL_POINTER {
					declSpec = declSpec[:len(declSpec)-1]
				} else {
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndirection, sym.ArgDetails, ast.CompileErrorOperands{})
				}
			case constants.DECL_POINTER:
				// &p on a *T p is a **T.
				if sym.PassBy == constants.PASSBY_REFERENCE {
					declSpec = append(declSpec, constants.DECL_POINTER)
				}
			}
		}
		sym.DeclarationSpecifiers = declSpec

		return
	} else if sym != nil && arg == nil && (len(sym.DeclarationSpecifiers) > 0 || len(sym.DereferenceOperations) > 0) && argTypeSignature.Type == ast.TYPE_ARRAY_ATOMIC {
		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0944.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSignature.Meta)
		newArrDetails := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		newArrDetails.Meta = arrDetails.Meta
		newArrDetails.Lengths = arrDetails.Lengths
		newArrDetails.Indexes = sym.Indexes
		newArrDetailsIdx := int(objmodel.GidIndex(int64(arrayGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newArrDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		return
	} else if sym != nil && arg == nil && (len(sym.DeclarationSpecifiers) > 0 || len(sym.DereferenceOperations) > 0) && argTypeSignature.Type == ast.TYPE_POINTER_ARRAY_ATOMIC {
		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0947.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSignature.Meta)
		newArrDetails := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		newArrDetails.Meta = arrDetails.Meta
		newArrDetails.Lengths = arrDetails.Lengths
		newArrDetails.Indexes = sym.Indexes
		newArrDetailsIdx := int(objmodel.GidIndex(int64(arrayGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newArrDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		for _, decl := range sym.DeclarationSpecifiers {
			// parr[i] indexes *parr, as in Go.
			if decl == constants.DECL_DEREF || decl == constants.DECL_INDEXING {
				symTypeSignature.IsDeref = true
			}
		}

		return
	} else if sym != nil && arg == nil && argTypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0950.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSignature.Meta)
		newArrDetails := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		newArrDetails.Meta = arrDetails.Meta
		newArrDetails.Lengths = arrDetails.Lengths
		newArrDetails.Indexes = sym.Indexes
		newArrDetailsIdx := int(objmodel.GidIndex(int64(arrayGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newArrDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		for _, decl := range sym.DeclarationSpecifiers {
			switch decl {
			case constants.DECL_DEREF, constants.DEREF_SLICE, constants.DEREF_ARRAY:
				symTypeSignature.IsDeref = true
			}
		}

		for _, deref := range sym.DereferenceOperations {
			switch deref {
			case constants.DEREF_SLICE:
				symTypeSignature.IsDeref = true
			}
		}

		return
	} else if sym != nil && arg == nil && argTypeSignature.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
		if len(sym.Indexes) > 0 && pointerIndexedBeforeDeref(sym.DeclarationSpecifiers) {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
		}
		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0953.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSignature.Meta)
		newArrDetails := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		newArrDetails.Meta = arrDetails.Meta
		newArrDetails.Lengths = arrDetails.Lengths
		newArrDetails.Indexes = sym.Indexes
		newArrDetailsIdx := int(objmodel.GidIndex(int64(arrayGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newArrDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		for _, decl := range sym.DeclarationSpecifiers {
			switch decl {
			case constants.DECL_DEREF, constants.DEREF_SLICE, constants.DEREF_ARRAY:
				symTypeSignature.IsDeref = true
			}
		}

		for _, deref := range sym.DereferenceOperations {
			switch deref {
			case constants.DEREF_SLICE:
				symTypeSignature.IsDeref = true
			}
		}

		return
	} else if sym != nil && arg == nil && argTypeSignature.Type == ast.TYPE_STRUCT || argTypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		structGid := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructAlloc()
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0956.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSignature.Meta)
		newStructDetails := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructGet(structGid)
		newStructDetails.StructType = structDetails.StructType
		newStructDetails.Fields = sym.Fields
		newStructDetailsIdx := int(objmodel.GidIndex(int64(structGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newStructDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		if argTypeSignature.Type == ast.TYPE_POINTER_STRUCT && len(sym.Fields) == 0 {
			for _, decl := range sym.DeclarationSpecifiers {
				if decl == constants.DECL_DEREF {
					symTypeSignature.IsDeref = true
				}
			}
		}

		if len(sym.Fields) > 0 {
			elt := sym.GetAssignmentElement(prgrm)

			declSpec := []int{}
			for c := 0; c < len(elt.DeclarationSpecifiers); c++ {
				switch elt.DeclarationSpecifiers[c] {
				case constants.DECL_INDEXING:
					if declSpec[len(declSpec)-1] == constants.DECL_ARRAY || declSpec[len(declSpec)-1] == constants.DECL_SLICE {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				case constants.DECL_DEREF:
					if declSpec[len(declSpec)-1] == constants.DECL_POINTER {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndirection, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				default:
					declSpec = append(declSpec, elt.DeclarationSpecifiers[c])
				}
			}

			elt.DeclarationSpecifiers = declSpec
		}

		return
	} else if sym != nil && arg == nil && (len(sym.DeclarationSpecifiers) > 1 || len(sym.DereferenceOperations) > 1) {
		sym.Name = argTypeSignature.Name
		sym.Package = argTypeSignature.Package
		sym.Type = types.Code(argTypeSignature.Meta)
		sym.Offset = argTypeSignature.Offset

		return
	} else if sym != nil && arg == nil {
		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = argTypeSignature.Meta
		symTypeSignature.Offset = argTypeSignature.Offset
		return
	} else if sym == nil && arg != nil {
		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = argTypeSignature.Meta
		symTypeSignature.Offset = argTypeSignature.Offset

		return
	}

	// This is for test-pointers when IsTypeAtomic includes types.IDENTIFIER
	// else if sym == nil && arg != nil && symTypeSignature.PassBy != 0 && arg.Type.IsPrimitive() {
	// 	symTypeSignature.Meta = int(arg.Type)
	// 	symTypeSignature.Offset = arg.Offset

	// 	return
	// }

	sym.Name = arg.Name
	sym.Offset = arg.Offset
	sym.Type = arg.Type

	// The parameter itself is processed too, at the site of its declaration.
	use := !copyArgFieldsDeclarationSite(prgrm, sym, arg)
	if use {
		// FIXME Maybe we can unify this later.
		if len(sym.Fields) > 0 {
			indexedFieldBaseDeref(sym, arg)
			fieldPointerDeref(sym, arg)
			elt := sym.GetAssignmentElement(prgrm)

			declSpec := []int{}
			for c := 0; c < len(elt.DeclarationSpecifiers); c++ {
				switch elt.DeclarationSpecifiers[c] {
				case constants.DECL_INDEXING:
					if declSpec[len(declSpec)-1] == constants.DECL_ARRAY || declSpec[len(declSpec)-1] == constants.DECL_SLICE {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				case constants.DECL_DEREF:
					if declSpec[len(declSpec)-1] == constants.DECL_POINTER {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndirection, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				default:
					declSpec = append(declSpec, elt.DeclarationSpecifiers[c])
				}
			}

			elt.DeclarationSpecifiers = declSpec
		} else {
			pointerArrayIndexed := pointerArrayIndexDeref(sym, arg)
			declSpec := make([]int, len(arg.DeclarationSpecifiers))

			for i, spec := range arg.DeclarationSpecifiers {
				declSpec[i] = spec
			}

			for _, spec := range sym.DeclarationSpecifiers {
				// checking if we need to remove or add cxcore.DECL_POINTERs
				// also we could be removing
				switch spec {
				case constants.DECL_INDEXING:
					if pointerArrayIndexed {
						// parr[i] indexes *parr, as in Go.
						declSpec = declSpec[:len(declSpec)-2]
						pointerArrayIndexed = false
					} else if declSpec[len(declSpec)-1] == constants.DECL_ARRAY || declSpec[len(declSpec)-1] == constants.DECL_SLICE {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				case constants.DECL_DEREF:
					if declSpec[len(declSpec)-1] == constants.DECL_POINTER {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndirection, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				case constants.DECL_POINTER:
					if use {
						// This function is also called so it assigns offset and other fields to signature parameters
						//
						declSpec = append(declSpec, constants.DECL_POINTER)
					}
				}
			}

			sym.DeclarationSpecifiers = declSpec
		}
	} else if sym.PassBy == constants.PASSBY_REFERENCE && len(sym.Fields) == 0 && len(sym.DeclarationSpecifiers) == 1 && sym.DeclarationSpecifiers[0] == constants.DECL_POINTER {
		// &T{...} takes the address of a temporary declared on its own line.
		sym.DeclarationSpecifiers = append(append([]int{}, arg.DeclarationSpecifiers...), constants.DECL_POINTER)
	} else {
		sym.DeclarationSpecifiers = arg.DeclarationSpecifiers
	}

	sym.StructType = arg.StructType

	// FIXME: In other processes like ProcessSymbolFields the symbol is assigned with lengths.
	// If we already have some lengths, we skip this. This needs to be fixed in the redesign of the cxgo.
	if len(sym.Lengths) == 0 {
		sym.Lengths = arg.Lengths
	}

	// sym.Lengths = arg.Lengths
	sym.Package = arg.Package
	sym.Size = arg.Size
	elementStruct := pointerArrayElementStruct(sym, arg)
	if elementStruct {
		// (*parr)[i] is a T, while parr has the size of a pointer.
		sym.Size = arg.StructType.GetStructSize(prgrm)
	}

	// Checking if it's a slice struct field. We'll do the same process as
	// below (as in the `arg.IsSlice` check), but the process differs in the
	// case of a slice struct field.
	assignElement := sym.GetAssignmentElement(prgrm)
	if (!arg.IsSlice() || hasDerefOp(sym, constants.DEREF_ARRAY)) && arg.StructType != nil && assignElement.IsSlice() && assignElement.ArraySliceDimensions() == 0 && assignElement != sym {
		for i, deref := range assignElement.DereferenceOperations {
			// The cxgo when reading `foo[5]` in postfix.go does not know if `foo`
			// is a slice or an array. At this point we now know it's a slice and we need
			// to change those dereferences to cxcore.DEREF_SLICE.
			if deref == constants.DEREF_ARRAY {
				assignElement.DereferenceOperations[i] = constants.DEREF_SLICE
			}
		}

		if len(assignElement.DereferenceOperations) > 0 && assignElement.DereferenceOperations[0] == constants.DEREF_POINTER {
			assignElement.DereferenceOperations = assignElement.DereferenceOperations[1:]
		}
	}

	if arg.ArraySliceMixed() && (hasDerefOp(sym, constants.DEREF_ARRAY) || hasDerefOp(sym, constants.DEREF_SLICE)) {
		indexDerefsOfLengths(sym, arg.Lengths)
	} else if arg.IsSlice() {
		if !hasDerefOp(sym, constants.DEREF_ARRAY) {
			// Then we're handling the slice itself, and we need to dereference it.
			sym.DereferenceOperations = append([]int{constants.DEREF_POINTER}, sym.DereferenceOperations...)
		} else {
			for i, deref := range sym.DereferenceOperations {
				// The cxgo when reading `foo[5]` in postfix.go does not know if `foo`
				// is a slice or an array. At this point we now know it's a slice and we need
				// to change those dereferences to cxcore.DEREF_SLICE.
				if deref == constants.DEREF_ARRAY {
					sym.DereferenceOperations[i] = constants.DEREF_SLICE
				}
			}
		}
	}

	if len(sym.Fields) > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0959.
		symField := prgrm.GetCXArgFromArray(sym.Fields[len(sym.Fields)-1])
		if sym.Type == types.POINTER && arg.StructType != nil {
			sym.PointerTargetType = symField.Type
		} else {
			sym.Type = symField.Type
			sym.PointerTargetType = symField.PointerTargetType
		}
	} else if elementStruct {
		sym.Type = arg.PointerTargetType
		sym.PointerTargetType = types.UNUSED
	} else {
		sym.Type = arg.Type
		sym.PointerTargetType = arg.PointerTargetType
	}
}

// ProcessSymbolFields copies the correct field values for the sym.Fields from their struct fields.
func ProcessSymbolFields(prgrm *ast.CXProgram, symTypeSignature, argTypeSignature *ast.CXTypeSignature) {
	if typeSignatureInvalid(prgrm, argTypeSignature) {
		// The fields of a value of no valid type have no type either, so their
		// uses report nothing more.
		if symTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			for _, fieldIdx := range prgrm.GetCXArgFromArray(ast.CXArgumentIndex(symTypeSignature.Meta)).Fields {
				prgrm.GetCXArgFromArray(fieldIdx).Type = types.INVALID
			}
		}
		return
	}
	var argStructType *ast.CXStruct
	if argTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0960.
		arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argTypeSignature.Meta))
		argStructType = arg.StructType
	} else if argTypeSignature.Type == ast.TYPE_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0961.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSignature.Meta)
		argStructType = structDetails.StructType
	} else if argTypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0962.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSignature.Meta)
		argStructType = structDetails.StructType
	} else {
		symbolFieldAccessPrimitiveCheck(prgrm, symTypeSignature, argTypeSignature)
		return
	}

	// nil for a struct signature.
	var sym *ast.CXArgument
	var symFields []ast.CXArgumentIndex
	if symTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0964.
		sym = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(symTypeSignature.Meta))
		symFields = sym.Fields
	} else if symTypeSignature.Type == ast.TYPE_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0965.
		symStructDetails := prgrm.GetCXTypeSignatureStructFromArray(symTypeSignature.Meta)
		symFields = symStructDetails.Fields
	} else if symTypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0966.
		symStructDetails := prgrm.GetCXTypeSignatureStructFromArray(symTypeSignature.Meta)
		symFields = symStructDetails.Fields
	} else {
		// panic("type is type cxargument deprecate\n\n")
		// ProcessSymbolFields is only needed for struct types
		// So we return from here
		return
	}

	if len(symFields) > 0 {
		if argStructType == nil || len(argStructType.Fields) == 0 {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeNoFields, symTypeSignature.ArgDetails, ast.CompileErrorOperands{int64(symTypeSignature.Name)})
			return
		}

		// checking if fields do exist in their StructType
		// and assigning that StructType to the sym.Field
		strct := argStructType
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0967.
		strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
		if err != nil {
			panic(err)
		}

		for fieldNumber, fldIdx := range symFields {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0968.
			field := prgrm.GetCXArgFromArray(fldIdx)
			if inFld, err := strct.GetField(prgrm, field.Name); err == nil {
				if inFld.GetStructType(prgrm) != nil {
					field.StructType = strct
					strct = inFld.GetStructType(prgrm)
				} else if fieldNumber < len(symFields)-1 {
					symbolFieldsAfterNonStruct(prgrm, field, inFld, symFields[fieldNumber+1:])
					return
				}
			} else {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0969.
				methodName := prgrm.GetCXArgFromArray(symFields[len(symFields)-1]).Name
				receiverType := strct.Name

				if methodIdx, methodErr := strctPkg.GetMethod(prgrm, receiverType, methodName); methodErr == nil {
					// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
					// Now an arena getter, site S0970.
					method := prgrm.GetFunctionFromArray(methodIdx)
					methodOutputs := method.GetOutputs(prgrm)
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S0971.
					methodOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(methodOutputs[0])

					var methodOutputArg *ast.CXArgument
					if methodOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S0973.
						methodOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(methodOutputTypeSig.Meta))

						field.Type = methodOutputArg.Type
						field.PointerTargetType = methodOutputArg.PointerTargetType
					} else {
						field.Type = methodOutputTypeSig.GetType(prgrm)
					}
				} else {
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeMessage, field.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})
					// The field has no type, so its uses report nothing more.
					field.Type = types.INVALID
				}
			}
		}

		strct = argStructType
		// then we copy all the type struct fields
		// to the respective symFields
		for _, nameFldIdx := range symFields {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0974.
			nameField := prgrm.GetCXArgFromArray(nameFldIdx)
			if nameField.StructType != nil {
				strct = nameField.StructType
			}

			for _, typeSignatureIdx := range strct.Fields {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0975.
				typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)

				if typeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					fieldIdx := typeSignature.Meta
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1740.
					field := prgrm.GetCXArg(ast.CXArgumentIndex(fieldIdx))

					if nameField.Name == field.Name {
						symbolFieldTypeSet(nameField, field, sym)
						if field.StructType != nil {
							strct = field.StructType
						}
						break
					}
				} else if field, ok := typeSignatureArgument(prgrm, typeSignature); ok && nameField.Name == typeSignature.Name && symbolFieldCompound(typeSignature, &field) {
					// A field of a pointer to an array or slice, or of an array of
					// arrays, has no case of its own below.
					field.PassBy = typeSignature.PassBy
					symbolFieldTypeSet(nameField, &field, sym)
					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_ATOMIC {
					nameField.Type = types.Code(typeSignature.Meta)
					nameField.StructType = nil
					nameField.Size = typeSignature.GetSize(prgrm, false)

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_BASIC}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_BASIC}
					}

					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_POINTER_ATOMIC {
					nameField.Type = types.Code(typeSignature.Meta)
					nameField.StructType = nil
					nameField.Size = types.POINTER_SIZE
					specs := []int{constants.DECL_BASIC, constants.DECL_POINTER}
					if len(nameField.DeclarationSpecifiers) > 0 {
						specs = append(specs, nameField.DeclarationSpecifiers[1:]...)
					}
					nameField.DeclarationSpecifiers = specs

					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_ARRAY_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0976.
					typeSignatureArray := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
					nameField.Type = types.Code(typeSignatureArray.Meta)
					nameField.StructType = nil
					nameField.Size = types.Code(typeSignatureArray.Meta).Size()
					nameField.Lengths = typeSignatureArray.Lengths
					if sym != nil {
						sym.Lengths = typeSignatureArray.Lengths
					}

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_BASIC, constants.DECL_ARRAY}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_BASIC}
					}
					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_SLICE_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0977.
					sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
					nameField.Type = types.Code(sliceDetails.Meta)
					nameField.StructType = nil
					nameField.Size = nameField.Type.Size()
					nameField.Lengths = sliceDetails.Lengths
					if sym != nil {
						sym.Lengths = sliceDetails.Lengths
					}

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append(declarationSpecifiersSlice(len(sliceDetails.Lengths)), nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_BASIC}
					}

					// Each index of a multi-dimensional slice field reads the next slice.
					if len(sliceDetails.Lengths) > 1 && hasDerefOp(nameField, constants.DEREF_ARRAY) {
						for i, deref := range nameField.DereferenceOperations {
							if deref == constants.DEREF_ARRAY {
								nameField.DereferenceOperations[i] = constants.DEREF_SLICE
							}
						}
					} else {
						nameField.DereferenceOperations = append([]int{constants.DEREF_POINTER}, nameField.DereferenceOperations...)
					}

					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_STRUCT {
					// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
					// Now an arena getter, site S0978.
					structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
					nameField.Type = types.STRUCT
					nameField.Size = typeSignature.GetSize(prgrm, false)
					nameField.StructType = structDetails.StructType
					if nameField.StructType != nil {
						strct = nameField.StructType
					}

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_STRUCT}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_STRUCT}
					}

					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_POINTER_STRUCT {
					// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
					// Now an arena getter, site S0979.
					structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
					nameField.Type = types.POINTER
					nameField.Size = typeSignature.GetSize(prgrm, false)
					nameField.StructType = structDetails.StructType
					if nameField.StructType != nil {
						strct = nameField.StructType
					}

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_STRUCT, constants.DECL_POINTER}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_STRUCT}
					}

					break
				}

				nameField.Offset += typeSignature.Offset
			}
		}
	}
}

// GetGlobalSymbol tries to retrieve `ident` from `symPkg`'s globals if `ident` is not found in the local scope.
func GetGlobalSymbol(prgrm *ast.CXProgram, symbolsData *SymbolsData, symPkg *ast.CXPackage, ident stringpool.StringGid, details ast.CXArgumentDebugGid) {
	_, err := lookupSymbol(prgrm, symPkg.Name, ident, symbolsData)
	if err != nil {
		if glbl, err := symPkg.GetGlobal(prgrm, ident); err == nil {
			scopeGlobalsUse(prgrm, symPkg, ident, details)
			lastIdx := len(*&symbolsData.symbolsIndex) - 1

			// add name to symbols
			symbolsData.symbols = append(symbolsData.symbols, glbl.Index)
			(symbolsData.symbolsIndex)[lastIdx][SymbolsDataKey{SymbolsDataKeyPackage: symPkg.Name, SymbolsDataKeyName: ident}] = len(symbolsData.symbols) - 1
		}
	}
}

// exprLine returns the file and line of the CX_LINE marker before exprs[currIndex].
// It falls back to the parser position, which is the end of the function once the body is parsed.
func exprLine(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) (ast.SourceFileGid, int) {
	if cxLine, err := prgrm.GetPreviousCXLine(exprs, currIndex); err == nil && cxLine.LineNumber > 0 {
		return cxLine.FileName, cxLine.LineNumber
	}
	return CurrentFile, LineNo
}

// releaseFunctionSymbols releases the signatures that lookupSymbol made for
// function names, with the argument each names and its debug record. Only
// the lookup that made one ever reads it.
func releaseFunctionSymbols(prgrm *ast.CXProgram, symbolsData *SymbolsData) {
	for _, typeSigIdx := range symbolsData.functionSymbols {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1777.
		typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)
		argDetails := typeSig.ArgDetails
		prgrm.ReleaseCXArg(ast.CXArgumentIndex(typeSig.Meta))
		prgrm.ReleaseCXTypeSignature(typeSigIdx)
		prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugRelease(argDetails)
	}
	symbolsData.functionSymbols = nil
}

// ProcessLoopJumpsOutsideLoop reports a break or continue that no loop
// resolved, since IterationExpressions turns every one it owns into a jmp.
func ProcessLoopJumpsOutsideLoop(prgrm *ast.CXProgram, exprs []ast.CXExpression) {
	for i, expr := range exprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		var keyword string
		if expr.IsBreak(prgrm) {
			keyword = "break"
		} else if expr.IsContinue(prgrm) {
			keyword = "continue"
		} else {
			continue
		}
		file, line := CurrentFile, LineNo
		if i > 0 && exprs[i-1].Type == ast.CX_LINE {
			if cxLine, err := prgrm.GetCXLine(exprs[i-1].Index); err == nil {
				file, line = cxLine.FileName, cxLine.LineNumber
			}
		}
		prgrm.CXProgramCompileErrorAddLine(ast.CompileErrorCodeLoopJumpOutsideLoop, file, line, ast.CompileErrorOperands{int64(prgrm.CXProgramNameIntern(keyword))})
	}
}

// typeSignatureUndefined reports whether the type of typeSig is not known
// yet, as for a temporary variable before its operator is typed.
func typeSignatureUndefined(prgrm *ast.CXProgram, typeSig *ast.CXTypeSignature) bool {
	if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		return prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta)).GetAssignmentElement(prgrm).Type == types.UNDEFINED
	}
	return typeSig.Type == ast.TYPE_UNUSED
}

// symbolFieldAccessPrimitiveCheck errors on a field of a symbol whose
// declared type is not a struct, as in x.y = 3 for an i32 x.
func symbolFieldAccessPrimitiveCheck(prgrm *ast.CXProgram, symTypeSignature, argTypeSignature *ast.CXTypeSignature) {
	if symTypeSignature.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return
	}
	sym := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(symTypeSignature.Meta))
	if len(sym.Fields) == 0 {
		return
	}
	prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodePrimitiveFieldAccess, symTypeSignature.ArgDetails, ast.CompileErrorOperands{int64(symTypeSignature.Name), prgrm.CXProgramCompileErrorText(ast.GetFormattedType(prgrm, argTypeSignature))})
}

// checkSliceBuiltinTypes checks the element and result types of append,
// insert, resize, remove and copy against the type of their slice.
func checkSliceBuiltinTypes(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expression, err := prgrm.GetCXAtomicOpFromExpressions(exprs, currIndex)
	if err != nil {
		panic(err)
	}
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	if expressionOperator == nil || !expressionOperator.IsBuiltIn() {
		return
	}
	opcode := expressionOperator.AtomicOPCode
	var elementInput int
	switch opcode {
	case constants.OP_APPEND:
		elementInput = 1
	case constants.OP_INSERT:
		elementInput = 2
	case constants.OP_RESIZE, constants.OP_REMOVE, constants.OP_COPY:
		elementInput = -1
	default:
		return
	}
	inputs := expression.GetInputs(prgrm)
	if len(inputs) == 0 {
		return
	}
	sliceTypeSig := prgrm.GetCXTypeSignatureFromArray(inputs[0])
	sliceType := ast.GetFormattedType(prgrm, sliceTypeSig)
	if !strings.HasPrefix(sliceType, "[]") {
		return
	}
	literal := opcode == constants.OP_APPEND && strings.HasPrefix(prgrm.CXProgramNameText(sliceTypeSig.Name), constants.SLICE_LITERAL_PREFIX)
	operandTypeCheck := func(typeSigIdx ast.CXTypeSignatureIndex, expectedType string) {
		typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)
		if typeSignatureUndefined(prgrm, typeSig) {
			return
		}
		temporary := IsTempVar(prgrm, typeSig.Name)
		receivedType := ast.GetFormattedType(prgrm, typeSig)
		if receivedType == expectedType {
			return
		}
		file, line := exprLine(prgrm, exprs, currIndex)
		column := prgrm.CXProgramCompileErrorColumn(file, line)
		if literal {
			// The slice of a slice literal element is at the element.
			if sliceFile, sliceLine, sliceColumn := ast.CXArgumentDebugPosition(prgrm, sliceTypeSig.ArgDetails); sliceLine > 0 {
				file, line, column = sliceFile, sliceLine, sliceColumn
			}
			prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeSliceLiteralElementType, file, line, column, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(receivedType), prgrm.CXProgramCompileErrorText(expectedType)})
			return
		}
		if sigFile, sigLine, sigColumn := ast.CXArgumentDebugPosition(prgrm, typeSig.ArgDetails); sigLine > 0 && !temporary {
			file, line, column = sigFile, sigLine, sigColumn
		}
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeSliceElementType, file, line, column, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(ast.NativeNames.NativeNameIndexCodeText(opcode)), prgrm.CXProgramCompileErrorText(expectedType), prgrm.CXProgramCompileErrorText(receivedType)})
	}
	if elementInput > 0 && elementInput < len(inputs) {
		operandTypeCheck(inputs[elementInput], sliceType[len("[]"):])
	}
	if opcode == constants.OP_COPY && len(inputs) > 1 {
		operandTypeCheck(inputs[1], sliceType)
		return
	}
	outputs := expression.GetOutputs(prgrm)
	if len(outputs) == 1 && !IsTempVar(prgrm, prgrm.GetCXTypeSignatureFromArray(outputs[0]).Name) {
		operandTypeCheck(outputs[0], sliceType)
	}
}

// declarationSpecifiersSlice is DECL_BASIC and one DECL_SLICE per dimension,
// so st.m on an m [][]i32 field is [][]i32 and st.m[0] is []i32.
func declarationSpecifiersSlice(dimensions int) []int {
	specs := make([]int, 1, dimensions+1)
	specs[0] = constants.DECL_BASIC
	for range dimensions {
		specs = append(specs, constants.DECL_SLICE)
	}
	return specs
}

// incDecOperandMissing reports whether exprAtomicOp is an i++ or i-- that has
// no 1 yet, which PostfixExpressionIncDec leaves out.
func incDecOperandMissing(prgrm *ast.CXProgram, exprAtomicOp *ast.CXAtomicOperator) bool {
	operator := prgrm.GetFunctionFromArray(exprAtomicOp.Operator)
	if operator == nil || (operator.AtomicOPCode != constants.OP_ADD && operator.AtomicOPCode != constants.OP_SUB) {
		return false
	}
	inputs, outputs := exprAtomicOp.GetInputs(prgrm), exprAtomicOp.GetOutputs(prgrm)
	if len(inputs) != 1 || len(outputs) != 1 || prgrm.GetCXTypeSignatureFromArray(inputs[0]).Name != prgrm.GetCXTypeSignatureFromArray(outputs[0]).Name {
		return false
	}
	return true
}

// incDecOperandAdd gives i++ and i-- the literal 1 of the type of i, which is
// known once the inputs are processed (skycoin/cx#450).
func incDecOperandAdd(prgrm *ast.CXProgram, exprAtomicOp *ast.CXAtomicOperator) {
	var one [8]byte
	typeCode := operatorOperandType(prgrm, exprAtomicOp.GetInputs(prgrm)[0])
	switch typeCode {
	case types.I8:
		types.Write_i8(one[:], 0, 1)
	case types.I16:
		types.Write_i16(one[:], 0, 1)
	case types.I64:
		types.Write_i64(one[:], 0, 1)
	case types.UI8:
		types.Write_ui8(one[:], 0, 1)
	case types.UI16:
		types.Write_ui16(one[:], 0, 1)
	case types.UI32:
		types.Write_ui32(one[:], 0, 1)
	case types.UI64:
		types.Write_ui64(one[:], 0, 1)
	case types.F32:
		types.Write_f32(one[:], 0, 1)
	case types.F64:
		types.Write_f64(one[:], 0, 1)
	default:
		typeCode = types.I32
		types.Write_i32(one[:], 0, 1)
	}
	literal := WritePrimary(prgrm, typeCode, one[:typeCode.Size()], false)
	exprAtomicOp.AddInput(prgrm, ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, literal))
}

// checkOperatorSameType reports an operator whose operands differ in type at
// the line of the operator, not at the end of the function (skycoin/cx#450).
func checkOperatorSameType(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expression := &prgrm.CXAtomicOps[exprs[currIndex].Index]
	operator := prgrm.GetFunctionFromArray(expression.Operator)
	if operator == nil || !ast.IsOperator(operator.AtomicOPCode) || len(expression.GetOutputs(prgrm)) == 0 || expressionOperandInvalid(prgrm, expression) {
		return
	}
	if err := checkSameNativeType(prgrm, &exprs[currIndex]); err != nil {
		file, line := exprLine(prgrm, exprs, currIndex)
		prgrm.CXProgramCompileErrorAddLine(ast.CompileErrorCodeMessage, file, line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})
	}
}

// shortDeclarationStructValue returns the type of the struct or of the field
// that a struct value read gives a short declaration, as in x := s.f.
func shortDeclarationStructValue(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) (types.Code, types.Code, types.Pointer, *ast.CXStruct) {
	details := prgrm.GetCXTypeSignatureStructFromArray(sig.Meta)
	if len(details.Fields) > 0 {
		field := prgrm.GetCXArg(details.Fields[len(details.Fields)-1]).GetAssignmentElement(prgrm)
		if len(field.Lengths) > len(field.Indexes) {
			panic("type is not known")
		}
		return field.Type, field.PointerTargetType, ast.GetArgSize(prgrm, field), field.StructType
	}
	if sig.Type == ast.TYPE_POINTER_STRUCT && !sig.IsDeref {
		panic("type is not known")
	}
	return types.STRUCT, 0, details.StructType.GetStructSize(prgrm), details.StructType
}

// shortDeclarationStructTypeSet gives arg the struct type of a struct value.
func shortDeclarationStructTypeSet(arg *ast.CXArgument, argType types.Code, structType *ast.CXStruct) {
	if structType == nil || argType != types.STRUCT {
		return
	}
	arg.StructType = structType
	arg.DeclarationSpecifiers = []int{constants.DECL_STRUCT}
}

// functionCallArguments joins the expressions of every argument and marks the
// last expression of each, which gives the value of the argument.
func functionCallArguments(arguments [][]ast.CXExpression) ([]ast.CXExpression, []bool) {
	var args []ast.CXExpression
	var argumentLast []bool
	for _, argument := range arguments {
		last := len(argument) - 1
		for last > 0 && argument[last].Type == ast.CX_LINE {
			last--
		}
		for i := range argument {
			args = append(args, argument[i])
			argumentLast = append(argumentLast, i == last)
		}
	}
	return args, argumentLast
}

// indexedFieldBaseDeref fixes the base of p[i].f before its fields are typed.
// A *[N]T reads through p before it indexes, a [N]*T or []*T keeps its
// pointer stride.
func indexedFieldBaseDeref(sym *ast.CXArgument, declared *ast.CXArgument) {
	specs := declared.DeclarationSpecifiers
	if pointerArrayElementPointerDeref(sym, declared) || pointerArrayIndexDeref(sym, declared) || len(sym.Indexes) == 0 || len(specs) < 2 || len(sym.DereferenceOperations) == 0 || sym.DereferenceOperations[0] != constants.DEREF_ARRAY {
		return
	}
	outer := specs[len(specs)-1]
	if (outer == constants.DECL_ARRAY || outer == constants.DECL_SLICE) && specs[len(specs)-2] == constants.DECL_POINTER {
		if len(sym.DeclarationSpecifiers) > 0 && sym.DeclarationSpecifiers[0] == constants.DECL_BASIC {
			sym.DeclarationSpecifiers = append(append([]int{}, specs...), sym.DeclarationSpecifiers[1:]...)
		}
	}
}

// pointerArrayElementStruct reports whether sym reads one whole struct element
// of the *[N]T or *[N][M]T declared, as (*p)[i] and p[i][j] do.
func pointerArrayElementStruct(sym *ast.CXArgument, declared *ast.CXArgument) bool {
	specs := declared.DeclarationSpecifiers
	if declared.StructType == nil || len(sym.Fields) > 0 || len(specs) < 3 || specs[0] != constants.DECL_STRUCT || specs[len(specs)-1] != constants.DECL_POINTER {
		return false
	}
	dims := pointerArrayDimensions(specs[1 : len(specs)-1])
	return dims == len(specs)-2 && len(sym.Indexes) == dims && hasDerefOp(sym, constants.DEREF_POINTER)
}

// pointerArrayDimensions returns how many specifiers of specs are arrays, or
// -1 when one is not.
func pointerArrayDimensions(specs []int) int {
	for _, spec := range specs {
		if spec != constants.DECL_ARRAY {
			return -1
		}
	}
	return len(specs)
}

// pointerArrayIndexDeref makes p[i] on a *[N]T read through p before it
// indexes, as Go does, and reports whether p is so indexed.
func pointerArrayIndexDeref(sym *ast.CXArgument, declared *ast.CXArgument) bool {
	specs := declared.DeclarationSpecifiers
	if len(sym.Indexes) == 0 || len(specs) < 2 || len(sym.DereferenceOperations) == 0 || sym.DereferenceOperations[0] != constants.DEREF_ARRAY {
		return false
	}
	if specs[len(specs)-1] != constants.DECL_POINTER || specs[len(specs)-2] != constants.DECL_ARRAY {
		return false
	}
	if !hasDerefOp(sym, constants.DEREF_POINTER) {
		sym.DereferenceOperations = append([]int{constants.DEREF_POINTER}, sym.DereferenceOperations...)
	}
	return true
}

// pointerArrayElementPointerDeref makes p[i].f on a *[N]*T and p[i][j].f on a
// *[N][M]*T read through p and then the element, with a pointer stride.
func pointerArrayElementPointerDeref(sym *ast.CXArgument, declared *ast.CXArgument) bool {
	specs := declared.DeclarationSpecifiers
	if len(sym.Fields) == 0 || len(specs) < 4 || specs[1] != constants.DECL_POINTER || specs[len(specs)-1] != constants.DECL_POINTER {
		return false
	}
	dims := pointerArrayDimensions(specs[2 : len(specs)-1])
	if dims < 1 || len(sym.Indexes) != dims {
		return false
	}
	ops := sym.DereferenceOperations
	if len(ops) > 0 && ops[0] == constants.DEREF_POINTER {
		ops = ops[1:]
	}
	if len(ops) != dims || pointerArrayDimensions(pointerArrayDerefSpecs(ops)) != dims {
		return false
	}
	sym.DereferenceOperations = append(append([]int{constants.DEREF_POINTER}, ops...), constants.DEREF_POINTER)
	if len(sym.DeclarationSpecifiers) > 0 && sym.DeclarationSpecifiers[0] == constants.DECL_BASIC {
		sym.DeclarationSpecifiers = append(append([]int{}, specs...), sym.DeclarationSpecifiers[1:]...)
	}
	return true
}

// pointerArrayDerefSpecs maps array dereferences to array specifiers, so
// pointerArrayDimensions can count them.
func pointerArrayDerefSpecs(ops []int) []int {
	specs := make([]int, len(ops))
	for i, op := range ops {
		specs[i] = constants.DECL_INDEXING
		if op == constants.DEREF_ARRAY {
			specs[i] = constants.DECL_ARRAY
		}
	}
	return specs
}

// shortDeclarationReference reports whether the value sig of a short
// declaration is &v of a struct or atomic v, so the variable is a pointer.
func shortDeclarationReference(prgrm *ast.CXProgram, sig *ast.CXTypeSignature, argType types.Code) bool {
	if ast.CXTypeSignaturePassBy(prgrm, sig) != constants.PASSBY_REFERENCE || argType == types.POINTER {
		return false
	}
	if sig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta)).GetAssignmentElement(prgrm)
		return len(arg.Lengths) == 0 && !arg.IsPointer()
	}
	return sig.Type == ast.TYPE_ATOMIC || sig.Type == ast.TYPE_STRUCT
}

// shortDeclarationPointerSet makes arg, typed as the value v of x := &v, a
// pointer to it, as DeclarationSpecifiers does for *T.
func shortDeclarationPointerSet(arg *ast.CXArgument) {
	base := constants.DECL_BASIC
	if arg.StructType != nil {
		base = constants.DECL_STRUCT
		arg.PointerTargetType = types.STRUCT
		arg.Type = types.POINTER
	} else if arg.Type == types.STR {
		arg.PointerTargetType = types.STR
		arg.Type = types.POINTER
	}
	arg.DeclarationSpecifiers = []int{base, constants.DECL_POINTER}
	arg.Size = types.POINTER_SIZE
}

// assignmentTargetFormattedType is the type of the target of x = v. The
// target takes the PassBy of &v, which says how to copy, not its type.
func assignmentTargetFormattedType(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) string {
	sigPassBy := sig.PassBy
	sig.PassBy = constants.PASSBY_VALUE
	defer func() { sig.PassBy = sigPassBy }()
	if sig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta))
		argPassBy := arg.PassBy
		arg.PassBy = constants.PASSBY_VALUE
		defer func() { arg.PassBy = argPassBy }()
	}
	return ast.GetFormattedType(prgrm, sig)
}

// pointerTypeOfLastField reports whether arg, an access such as a[i].p or
// t.p, has the pointer type of its last field p rather than being a pointer.
// CopyArgFields gives a pointer base the field type as PointerTargetType.
func pointerTypeOfLastField(prgrm *ast.CXProgram, arg *ast.CXArgument) bool {
	field := prgrm.GetCXArgFromArray(arg.Fields[len(arg.Fields)-1])
	return field.IsPointer() && arg.PointerTargetType != types.POINTER
}

// indexDerefsOfLengths makes each index of arg index a slice or an array as
// its dimension in lengths, outermost first, is a slice or an array, as
// a[i][j][k] on a [][2][]i32 indexes a slice, an array and a slice.
func indexDerefsOfLengths(arg *ast.CXArgument, lengths []types.Pointer) {
	index := 0
	for i, op := range arg.DereferenceOperations {
		if op != constants.DEREF_ARRAY && op != constants.DEREF_SLICE {
			continue
		}
		arg.DereferenceOperations[i] = constants.DEREF_ARRAY
		if index < len(lengths) && lengths[index] == 0 {
			arg.DereferenceOperations[i] = constants.DEREF_SLICE
		}
		index++
	}
}

// shortDeclarationType is the type of the value of x := v.
type shortDeclarationType struct {
	typ, pointerTargetType types.Code
	size                   types.Pointer
	specs                  []int
	lengths                []types.Pointer
	structType             *ast.CXStruct
}

// shortDeclarationCollectionType returns the type of the value sig names
// when it is an array or a slice, as s, m[0], r[1] and p.xs are.
// &a of such a value is a pointer to it.
func shortDeclarationCollectionType(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) (shortDeclarationType, bool) {
	reference := ast.CXTypeSignaturePassBy(prgrm, sig) == constants.PASSBY_REFERENCE
	t, ok := shortDeclarationCollectionValue(prgrm, sig, reference)
	if !ok || !reference {
		return t, ok
	}
	t.specs = append(t.specs, constants.DECL_POINTER)
	t.size = types.POINTER_SIZE
	if t.typ == types.STR || (t.structType != nil && t.typ != types.POINTER) {
		t.pointerTargetType, t.typ = t.typ, types.POINTER
	}
	return t, true
}

// shortDeclarationCollectionValue is shortDeclarationCollectionType for a
// value read by value.
func shortDeclarationCollectionValue(prgrm *ast.CXProgram, sig *ast.CXTypeSignature, reference bool) (shortDeclarationType, bool) {
	switch sig.Type {
	case ast.TYPE_ARRAY_ATOMIC, ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		pointer := sig.Type == ast.TYPE_POINTER_ARRAY_ATOMIC || sig.Type == ast.TYPE_POINTER_SLICE_ATOMIC
		if pointer && !sig.IsDeref {
			return shortDeclarationType{}, false
		}
		details := prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta)
		if len(details.Indexes) >= len(details.Lengths) {
			return shortDeclarationType{}, false
		}
		slice := sig.Type == ast.TYPE_SLICE_ATOMIC || sig.Type == ast.TYPE_POINTER_SLICE_ATOMIC
		lengths := append([]types.Pointer{}, details.Lengths[len(details.Indexes):]...)
		specs := []int{constants.DECL_BASIC}
		for i := len(lengths) - 1; i >= 0; i-- {
			if slice && lengths[i] == 0 {
				specs = append(specs, constants.DECL_SLICE)
			} else {
				specs = append(specs, constants.DECL_ARRAY)
			}
		}
		typ := types.Code(details.Meta)
		return shortDeclarationType{typ: typ, size: typ.Size(), specs: specs, lengths: lengths}, true
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		return shortDeclarationElementType(prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta)).GetAssignmentElement(prgrm), reference)
	case ast.TYPE_STRUCT, ast.TYPE_POINTER_STRUCT:
		fields := prgrm.GetCXTypeSignatureStructFromArray(sig.Meta).Fields
		if len(fields) == 0 {
			return shortDeclarationType{}, false
		}
		return shortDeclarationElementType(prgrm.GetCXArg(fields[len(fields)-1]).GetAssignmentElement(prgrm), reference)
	}
	return shortDeclarationType{}, false
}

// shortDeclarationElementType returns the type of the array or slice elt
// reads, whose specifiers CopyArgFields has stripped of its indexes. The
// caller adds the pointer of &a.
func shortDeclarationElementType(elt *ast.CXArgument, reference bool) (shortDeclarationType, bool) {
	specs := elt.DeclarationSpecifiers
	if reference && len(specs) > 0 && specs[len(specs)-1] == constants.DECL_POINTER {
		specs = specs[:len(specs)-1]
	}
	if len(specs) < 2 {
		return shortDeclarationType{}, false
	}
	if outer := specs[len(specs)-1]; outer != constants.DECL_ARRAY && outer != constants.DECL_SLICE {
		return shortDeclarationType{}, false
	}
	dims := 0
	for _, spec := range specs {
		switch spec {
		case constants.DECL_ARRAY, constants.DECL_SLICE:
			dims++
		case constants.DECL_INDEXING, constants.DECL_DEREF:
			return shortDeclarationType{}, false
		}
	}
	if len(elt.Lengths) < dims {
		return shortDeclarationType{}, false
	}
	return shortDeclarationType{
		typ:               elt.Type,
		pointerTargetType: elt.PointerTargetType,
		size:              elt.Size,
		specs:             append([]int{}, specs...),
		lengths:           append([]types.Pointer{}, elt.Lengths[len(elt.Lengths)-dims:]...),
		structType:        elt.StructType,
	}, true
}

// shortDeclarationCollectionSet gives the variable decl declares the array or
// slice type t, and makes assign read it again. It reports whether it could.
func shortDeclarationCollectionSet(prgrm *ast.CXProgram, decl, assign *ast.CXAtomicOperator, t shortDeclarationType, offset *types.Pointer) bool {
	declSig := prgrm.GetCXTypeSignatureFromArray(decl.GetOutputs(prgrm)[0])
	outSig := prgrm.GetCXTypeSignatureFromArray(assign.GetOutputs(prgrm)[0])
	if declSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE || outSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return false
	}
	declArg := prgrm.GetCXArg(ast.CXArgumentIndex(declSig.Meta))
	reserved := ast.GetArgSize(prgrm, declArg)
	declArg.Type, declArg.PointerTargetType, declArg.Size = t.typ, t.pointerTargetType, t.size
	declArg.DeclarationSpecifiers, declArg.Lengths, declArg.StructType = t.specs, t.lengths, t.structType
	if size := ast.GetArgSize(prgrm, declArg); size > reserved {
		declArg.Offset = *offset
		updateSymbolsTableOffsetAdd(prgrm, declSig, offset, size)
	}
	ast.RetypeCXTypeSignatureOfCXArg(prgrm, declSig, declArg)

	fn, err := prgrm.GetCurrentFunction()
	if err != nil {
		panic(err)
	}
	var sym *ast.CXArgument
	if declSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		sym = declArg
	}
	gcRootsDeclare(prgrm, fn, declSig, sym)

	outArg := prgrm.GetCXArg(ast.CXArgumentIndex(outSig.Meta))
	outArg.DeclarationSpecifiers, outArg.Lengths, outArg.DereferenceOperations = nil, nil, nil
	CopyArgFields(prgrm, outSig, declSig)
	if outSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		ProcessSlice(prgrm, ast.CXArgumentIndex(outSig.Meta))
	}
	return true
}

// typeSignatureInvalid reports whether typeSig is a types.INVALID placeholder,
// a value whose compile error is already recorded.
func typeSignatureInvalid(prgrm *ast.CXProgram, typeSig *ast.CXTypeSignature) bool {
	switch typeSig.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))
		return arg.Type == types.INVALID || arg.GetAssignmentElement(prgrm).Type == types.INVALID
	case ast.TYPE_ATOMIC, ast.TYPE_POINTER_ATOMIC:
		return types.Code(typeSig.Meta) == types.INVALID
	}
	return false
}

// expressionOperandInvalid reports whether an input or output of expression
// is a types.INVALID placeholder, or its operator stands for a missing function.
func expressionOperandInvalid(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator) bool {
	if functionInvalid(prgrm, prgrm.GetFunctionFromArray(expression.Operator)) {
		return true
	}
	for _, typeSigIdx := range expression.GetInputs(prgrm) {
		if typeSignatureInvalid(prgrm, prgrm.GetCXTypeSignatureFromArray(typeSigIdx)) {
			return true
		}
	}
	for _, typeSigIdx := range expression.GetOutputs(prgrm) {
		if typeSignatureInvalid(prgrm, prgrm.GetCXTypeSignatureFromArray(typeSigIdx)) {
			return true
		}
	}
	return false
}

// invalidFunctionIndex adds a function that stands for the missing function
// name, so that its call is typed. It takes any inputs and returns one
// types.INVALID result, and the type checks skip its calls.
func invalidFunctionIndex(prgrm *ast.CXProgram, pkgIdx ast.CXPackageIndex, name stringpool.StringGid) ast.CXFunctionIndex {
	fn := ast.MakeFunction(prgrm, name, CurrentFile, LineNo)
	fn.Package = pkgIdx
	_, input := ast.MakeArgument(prgrm, prgrm.CXProgramNameIntern("input"), CurrentFile, LineNo)
	fn.AddInput(prgrm, input.SetType(types.UNDEFINED))
	_, output := ast.MakeArgument(prgrm, prgrm.CXProgramNameIntern("output"), CurrentFile, LineNo)
	fn.AddOutput(prgrm, output.SetType(types.INVALID))
	return prgrm.AddFunctionInArray(fn)
}

// functionInvalid reports whether fn is a function of invalidFunctionIndex.
func functionInvalid(prgrm *ast.CXProgram, fn *ast.CXFunction) bool {
	if fn == nil || fn.IsBuiltIn() {
		return false
	}
	outputs := fn.GetOutputs(prgrm)
	return len(outputs) == 1 && typeSignatureInvalid(prgrm, prgrm.GetCXTypeSignatureFromArray(outputs[0]))
}

// dereferenceNonPointer records a use of a symbol, sym, that dereferences
// more pointers than its declaration, declared, has, as *x of an i32 x. The
// use then has types.INVALID, so its uses report nothing more.
func dereferenceNonPointer(prgrm *ast.CXProgram, sym *ast.CXTypeSignature, declared *ast.CXTypeSignature) bool {
	if sym.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return false
	}
	use := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sym.Meta))
	if len(use.Fields) > 0 || len(use.Indexes) > 0 {
		return false
	}
	var depth int
	switch declared.Type {
	case ast.TYPE_ATOMIC, ast.TYPE_ARRAY_ATOMIC, ast.TYPE_SLICE_ATOMIC, ast.TYPE_STRUCT:
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		specs := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(declared.Meta)).DeclarationSpecifiers
		for i := len(specs) - 1; i >= 0 && specs[i] == constants.DECL_POINTER; i-- {
			depth++
		}
	default:
		return false
	}
	for _, spec := range use.DeclarationSpecifiers {
		switch spec {
		case constants.DECL_BASIC:
		case constants.DECL_POINTER:
			depth++
		case constants.DECL_DEREF:
			depth--
			if depth < 0 {
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeDereferenceNonPointer, use.ArgDetails, ast.CompileErrorOperands{int64(StripNameNumber(prgrm, use.Name)), prgrm.CXProgramCompileErrorText(ast.GetFormattedType(prgrm, declared))})
				use.Type, use.Size = types.INVALID, 0
				return true
			}
		default:
			return false
		}
	}
	return false
}

// argumentIndexedElement reports whether sig names an element, tmp[i], of
// a literal LiteralCompound builds, which has its type from the literal.
func argumentIndexedElement(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) bool {
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return false
	}
	arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta))
	return len(arg.Indexes) > 0 && literalTypeCompound(arg)
}

// copyArgFieldsDeclarationSite reports whether sym sits where arg is declared,
// which a use on the line of the declaration, as in func f() (r [2]i32) { r[1] = 2 }, does not.
func copyArgFieldsDeclarationSite(prgrm *ast.CXProgram, sym *ast.CXArgument, arg *ast.CXArgument) bool {
	for _, spec := range sym.DeclarationSpecifiers {
		if spec == constants.DECL_INDEXING {
			// An element, as tmp[i] of a literal on the line of tmp is.
			return false
		}
	}
	_, symLine := ast.CXArgumentDebugFileLine(prgrm, sym.ArgDetails)
	_, argLine := ast.CXArgumentDebugFileLine(prgrm, arg.ArgDetails)
	if symLine != argLine {
		return false
	}
	if sym.ArgDetails == ast.CXArgumentDebugGidNil || arg.ArgDetails == ast.CXArgumentDebugGidNil {
		return true
	}
	symColumn := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(sym.ArgDetails).FileColumn
	argColumn := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(arg.ArgDetails).FileColumn
	return symColumn == argColumn
}

// symbolFieldsAfterNonStruct records the access of the fields after field,
// whose type inFld is not a struct, as h.c.radius of an i32 c, unless the
// type of field is invalid, as that of a struct of a failed import is. The
// fields after it then have no type, so their uses report nothing more.
func symbolFieldsAfterNonStruct(prgrm *ast.CXProgram, field *ast.CXArgument, inFld *ast.CXTypeSignature, after []ast.CXArgumentIndex) {
	if !typeSignatureInvalid(prgrm, inFld) {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodePrimitiveFieldAccess, prgrm.GetCXArgFromArray(after[0]).ArgDetails, ast.CompileErrorOperands{int64(field.Name), prgrm.CXProgramCompileErrorText(ast.GetFormattedType(prgrm, inFld))})
	}
	for _, fieldIdx := range after {
		prgrm.GetCXArgFromArray(fieldIdx).Type = types.INVALID
	}
}

// shortDeclarationPointerType returns the type of the value sig names when
// it is a variable of a pointer type, as p of a *str, **i32, *[]T or *T is,
// or &v of such a variable.
func shortDeclarationPointerType(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) (shortDeclarationType, bool) {
	var value ast.CXArgument
	switch sig.Type {
	case ast.TYPE_POINTER_ATOMIC, ast.TYPE_POINTER_STRUCT, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		var ok bool
		if value, ok = typeSignatureArgument(prgrm, sig); !ok {
			return shortDeclarationType{}, false
		}
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		arg := prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta))
		specs := arg.DeclarationSpecifiers
		if len(arg.Indexes) > 0 || len(arg.Fields) > 0 || len(arg.DereferenceOperations) > 0 || len(specs) < 2 || specs[len(specs)-1] != constants.DECL_POINTER {
			return shortDeclarationType{}, false
		}
		argumentTypeCopy(&value, arg)
		// &v of an argument has the pointer in its specifiers already.
		return shortDeclarationTypeOf(&value), true
	default:
		return shortDeclarationType{}, false
	}
	if ast.CXTypeSignaturePassBy(prgrm, sig) == constants.PASSBY_REFERENCE {
		DeclarationSpecifiers(&value, nil, constants.DECL_POINTER)
	}
	return shortDeclarationTypeOf(&value), true
}

// shortDeclarationTypeOf returns the type of arg.
func shortDeclarationTypeOf(arg *ast.CXArgument) shortDeclarationType {
	return shortDeclarationType{
		typ:               arg.Type,
		pointerTargetType: arg.PointerTargetType,
		size:              arg.Size,
		specs:             arg.DeclarationSpecifiers,
		lengths:           arg.Lengths,
		structType:        arg.StructType,
	}
}

// typeSignatureArgument returns the argument a declaration of the type of
// sig makes, for a signature that is not an argument and reads no index,
// field or dereference.
func typeSignatureArgument(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) (ast.CXArgument, bool) {
	var arg ast.CXArgument
	if sig.IsDeref {
		return arg, false
	}
	switch sig.Type {
	case ast.TYPE_ATOMIC, ast.TYPE_POINTER_ATOMIC:
		arg.Type, arg.Size = types.Code(sig.Meta), types.Code(sig.Meta).Size()
		arg.DeclarationSpecifiers = []int{constants.DECL_BASIC}
	case ast.TYPE_STRUCT, ast.TYPE_POINTER_STRUCT:
		details := prgrm.GetCXTypeSignatureStructFromArray(sig.Meta)
		if len(details.Fields) > 0 || details.StructType == nil {
			return arg, false
		}
		arg.Type, arg.StructType, arg.Size = types.STRUCT, details.StructType, details.StructType.GetStructSize(prgrm)
		arg.DeclarationSpecifiers = []int{constants.DECL_STRUCT}
	case ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		details := prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta)
		if len(details.Indexes) > 0 {
			return arg, false
		}
		arg.Type, arg.Size = types.Code(details.Meta), types.Code(details.Meta).Size()
		arg.DeclarationSpecifiers = []int{constants.DECL_BASIC}
		slice := sig.Type == ast.TYPE_SLICE_ATOMIC || sig.Type == ast.TYPE_POINTER_SLICE_ATOMIC
		for range details.Lengths {
			if slice {
				DeclarationSpecifiers(&arg, nil, constants.DECL_SLICE)
			} else {
				arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_ARRAY)
			}
		}
		if !slice {
			arg.Lengths = append([]types.Pointer(nil), details.Lengths...)
		}
	default:
		return arg, false
	}
	switch sig.Type {
	case ast.TYPE_POINTER_ATOMIC, ast.TYPE_POINTER_STRUCT, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		DeclarationSpecifiers(&arg, nil, constants.DECL_POINTER)
	}
	return arg, true
}

// symbolFieldTypeSet gives nameField, a field sym reads, the type of the
// struct field it names.
func symbolFieldTypeSet(nameField *ast.CXArgument, field *ast.CXArgument, sym *ast.CXArgument) {
	nameField.Type = field.Type
	nameField.Lengths = field.Lengths
	nameField.Size = field.Size
	nameField.PointerTargetType = field.PointerTargetType
	nameField.StructType = field.StructType

	if sym != nil {
		sym.Lengths = field.Lengths
	}

	if len(nameField.DeclarationSpecifiers) > 0 {
		nameField.DeclarationSpecifiers = append(append([]int(nil), field.DeclarationSpecifiers...), nameField.DeclarationSpecifiers[1:]...)
	} else {
		nameField.DeclarationSpecifiers = field.DeclarationSpecifiers
	}

	if field.IsSlice() {
		nameField.DereferenceOperations = append([]int{constants.DEREF_POINTER}, nameField.DereferenceOperations...)
	}
	if field.ArraySliceDimensions() > 0 {
		indexDerefsOfLengths(nameField, field.Lengths)
	}
	if field.SliceArrayDimensions() > 0 && hasDerefOp(nameField, constants.DEREF_ARRAY) {
		nameField.DereferenceOperations = nameField.DereferenceOperations[1:]
		indexDerefsOfLengths(nameField, field.Lengths)
	}

	nameField.PassBy = field.PassBy
}

// symbolFieldCompound reports whether the field of signature sig, made into
// field, is a pointer to an array or slice, or an array of arrays.
func symbolFieldCompound(sig *ast.CXTypeSignature, field *ast.CXArgument) bool {
	switch sig.Type {
	case ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		return true
	case ast.TYPE_ARRAY_ATOMIC:
		return len(field.Lengths) > 1
	}
	return false
}

// outputlessCallInvalidate reports expression, a call of operator, a
// function with no results, used as a value at file and line. The call
// becomes a stand in typed types.INVALID, so its uses report nothing more.
func outputlessCallInvalidate(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator, operator *ast.CXFunction, file ast.SourceFileGid, line int) *ast.CXFunction {
	prgrm.CXProgramCompileErrorAddLine(ast.CompileErrorCodeFunctionResultMissing, file, line, ast.CompileErrorOperands{int64(operator.Name)})
	expression.Operator = invalidFunctionIndex(prgrm, expression.Package, operator.Name)
	return prgrm.GetFunctionFromArray(expression.Operator)
}

// outputlessValueCheck reports the value exprs when it ends in a call of a
// function with no results, and makes the call a stand in typed
// types.INVALID, see outputlessCallInvalidate.
func outputlessValueCheck(prgrm *ast.CXProgram, exprs []ast.CXExpression) {
	valueCheck(prgrm, exprs, false)
}

// SingleValueCheck reports the value exprs when it ends in a call of a
// function with no results or with more than one, as Go does, and makes the
// call a stand in typed types.INVALID.
func SingleValueCheck(prgrm *ast.CXProgram, exprs []ast.CXExpression) {
	valueCheck(prgrm, exprs, true)
}

// valueCheck is outputlessValueCheck, and SingleValueCheck when single is
// true.
func valueCheck(prgrm *ast.CXProgram, exprs []ast.CXExpression, single bool) {
	for i := len(exprs) - 1; i >= 0; i-- {
		if exprs[i].Type == ast.CX_LINE {
			continue
		}
		expression := &prgrm.CXAtomicOps[exprs[i].Index]
		operator := prgrm.GetFunctionFromArray(expression.Operator)
		if operator == nil || exprs[i].IsMethodCall() || len(expression.GetOutputs(prgrm)) > 0 {
			return
		}
		results := operator.GetOutputs(prgrm)
		if len(results) == 1 || len(results) > 1 && !single {
			return
		}
		file, line := CurrentFile, LineNo
		if cxLine, err := prgrm.GetPreviousCXLine(exprs, i); err == nil && cxLine.LineNumber > 0 {
			file, line = cxLine.FileName, cxLine.LineNumber
		}
		if len(results) == 0 {
			outputlessCallInvalidate(prgrm, expression, operator, file, line)
			return
		}
		list := make([]string, len(results))
		for r, result := range results {
			sig := prgrm.GetCXTypeSignatureFromArray(result)
			list[r] = ast.GetFormattedType(prgrm, sig)
			if name := prgrm.CXProgramNameText(StripNameNumber(prgrm, sig.Name)); name != "" {
				list[r] = name + " " + list[r]
			}
		}
		prgrm.CXProgramCompileErrorAddLine(ast.CompileErrorCodeMultipleValueSingle, file, line, ast.CompileErrorOperands{int64(operator.Name), prgrm.CXProgramCompileErrorText(strings.Join(list, ", "))})
		expression.Operator = invalidFunctionIndex(prgrm, expression.Package, operator.Name)
		return
	}
}

// valueFormattedType is the type of the value sig gives, which is &p, a
// pointer to the pointer p, when sig is a pointer variable read by reference.
func valueFormattedType(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) string {
	typ := ast.GetFormattedType(prgrm, sig)
	if sig.PassBy != constants.PASSBY_REFERENCE || sig.IsDeref {
		return typ
	}
	switch sig.Type {
	case ast.TYPE_POINTER_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		return "*" + typ
	case ast.TYPE_POINTER_STRUCT:
		if len(prgrm.GetCXTypeSignatureStructFromArray(sig.Meta).Fields) == 0 {
			return "*" + typ
		}
	}
	return typ
}
