package actions

import (
	"slices"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin/src/cipher/encoder"
)

// ReturnExpressions stores the `Size` of the return arguments represented by `Expressions`.
// For example: `return foo() + bar()` is a set of 3 expressions and they represent a single return argument
type ReturnExpressions struct {
	Size int
	// Line and Column are where the return keyword is, or 0.
	Line   int
	Column int
	// OBJMODEL: []CXExpressionGid or CXExpressionGid, phase 5 #347.
	// Now objects held by value, site S0607.
	Expressions []ast.CXExpression
}

// IterationExpressions creates series of expressions that will create
// a for loop condition.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		initializeExprs - contains the initialization of the variables used
//						  for the condition of the iteration expression.
//	 conditionExprs - contains the condition of the iteration expression.
//	 incrementExprs - contains the increment expr for the variable used
//						 in the condition.
//	 statementExprs - contains the statements inside the iteration expression.
func IterationExpressions(prgrm *ast.CXProgram,
	initializeExprs []ast.CXExpression,
	conditionExprs []ast.CXExpression,
	incrementExprs []ast.CXExpression,
	statementExprs []ast.CXExpression) []ast.CXExpression {
	jmpFn := ast.Natives[constants.OP_JMP]

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0608.
	upExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0609.
	upExpr := ast.MakeAtomicOperatorExpression(prgrm, jmpFn)
	upExpressionIdx := upExpr.Index

	prgrm.CXAtomicOps[upExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	lastCondExpressionIdx := conditionExprs[len(conditionExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0613.
	lastCondExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastCondExpressionIdx].Operator)
	// A literal or variable condition is read in place, as in SelectionExpressions.
	// Running it would zero it as a declaration (skycoin-cx#20).
	loopConditionExprs := conditionExprs
	conditionInPlace := lastCondExpressionOperator == nil
	if conditionInPlace {
		loopConditionExprs = conditionExprs[:len(conditionExprs)-1]
	}

	// -2 for the cx line expression addition for up and down expr
	upLines := ((len(statementExprs) + len(incrementExprs) + len(loopConditionExprs) + 2) * -1) - 2
	downLines := 0

	trueArg := WritePrimary(prgrm, types.BOOL, encoder.Serialize(true), false)
	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, trueArg)
	prgrm.CXAtomicOps[upExpressionIdx].AddInput(prgrm, typeSigIdx)
	prgrm.CXAtomicOps[upExpressionIdx].ThenLines = upLines
	prgrm.CXAtomicOps[upExpressionIdx].ElseLines = downLines

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0611.
	downExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0612.
	downExpr := ast.MakeAtomicOperatorExpression(prgrm, jmpFn)
	downExpressionIdx := downExpr.Index

	prgrm.CXAtomicOps[downExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	lastCondExpressionOperatorOutputs := lastCondExpressionOperator.GetOutputs(prgrm)
	if len(prgrm.CXAtomicOps[lastCondExpressionIdx].GetOutputs(prgrm)) < 1 {
		if len(lastCondExpressionOperatorOutputs) == 0 {
			// At the condition, which the parser rule of the loop makes the token position.
			list := &prgrm.CxIrState.CompileErrorList
			prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeLoopConditionNoValue, list.CompileErrorListTokenFile, list.CompileErrorListTokenLine, list.CompileErrorListTokenColumn, ast.CompileErrorOperands{})
			// The condition reads a placeholder, so it reports nothing more.
			lastCondExpressionOperatorOutputs = []ast.CXTypeSignatureIndex{invalidPlaceholderTypeSig(prgrm, ast.CXPackageIndex(pkg.Index))}
		}
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0614.
		lastCondExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastCondExpressionOperatorOutputs[0])

		var lastCondExpressionOperatorOutputType types.Code
		var typeSigIdx ast.CXTypeSignatureIndex
		if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0615.
			lastCondExpressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastCondExpressionOperatorOutputTypeSig.Meta))
			lastCondExpressionOperatorOutputType = lastCondExpressionOperatorOutputArg.Type
			_, predicate := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
			predicate.SetType(lastCondExpressionOperatorOutputType)
			predicate.Package = ast.CXPackageIndex(pkg.Index)
			predicate.PreviouslyDeclared = true

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, predicate)
		} else if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(lastCondExpressionOperatorOutputTypeSig)
			newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			newTypeSig.Package = ast.CXPackageIndex(pkg.Index)
			newTypeSig.Offset = types.Pointer(0)
			typeSigIdx = newTypeSig.Index
		} else if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(lastCondExpressionOperatorOutputTypeSig)
			newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			newTypeSig.Package = ast.CXPackageIndex(pkg.Index)
			newTypeSig.Offset = types.Pointer(0)
			typeSigIdx = newTypeSig.Index
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[lastCondExpressionIdx].AddOutput(prgrm, typeSigIdx)
		prgrm.CXAtomicOps[downExpressionIdx].AddInput(prgrm, typeSigIdx)
	} else if conditionInPlace {
		prgrm.CXAtomicOps[downExpressionIdx].AddInput(prgrm, prgrm.CXAtomicOps[lastCondExpressionIdx].GetOutputs(prgrm)[0])
	} else {
		predicateTypeSigIdx := prgrm.CXAtomicOps[lastCondExpressionIdx].GetOutputs(prgrm)[0]
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0624.
		predicateTypeSig := prgrm.GetCXTypeSignatureFromArray(predicateTypeSigIdx)
		var predicateIdx ast.CXArgumentIndex
		if predicateTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			predicateIdx = ast.CXArgumentIndex(predicateTypeSig.Meta)
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1675.
			prgrm.GetCXArg(ast.CXArgumentIndex(predicateIdx)).Package = ast.CXPackageIndex(pkg.Index)
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1676.
			prgrm.GetCXArg(ast.CXArgumentIndex(predicateIdx)).PreviouslyDeclared = true
		} else if predicateTypeSig.Type == ast.TYPE_ATOMIC || predicateTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// do nothing
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[downExpressionIdx].AddInput(prgrm, predicateTypeSigIdx)
	}

	thenLines := 0
	// + 1 for the cx line expression addition for the down expr jmp
	elseLines := len(incrementExprs) + len(statementExprs) + 1 + 1

	// A resolved break or continue becomes a true jmp, so an enclosing loop
	// does not retarget it (skycoin-cx#39).
	for i, statementExpr := range statementExprs {
		if statementExpr.Type == ast.CX_LINE {
			continue
		}
		statementExpressionIdx := statementExpr.Index
		if statementExpr.IsBreak(prgrm) {
			prgrm.CXAtomicOps[statementExpressionIdx].ThenLines = elseLines - i - 1
			prgrm.CXAtomicOps[statementExpressionIdx].Operator = prgrm.AddNativeFunctionInArray(jmpFn)
		} else if statementExpr.IsContinue(prgrm) {
			prgrm.CXAtomicOps[statementExpressionIdx].ThenLines = len(statementExprs) - i - 1
			prgrm.CXAtomicOps[statementExpressionIdx].Operator = prgrm.AddNativeFunctionInArray(jmpFn)
		}
	}

	prgrm.CXAtomicOps[downExpressionIdx].ThenLines = thenLines
	prgrm.CXAtomicOps[downExpressionIdx].ElseLines = elseLines

	exprs := initializeExprs
	exprs = append(exprs, loopConditionExprs...)
	exprs = append(exprs, *downExprCXLine, *downExpr)
	exprs = append(exprs, statementExprs...)
	exprs = append(exprs, incrementExprs...)
	exprs = append(exprs, *upExprCXLine, *upExpr)

	DefineNewScope(exprs)

	return exprs
}

// trueJmpExpressions makes a true jump expression for BREAKS and CONTINUES.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		opcode - the opcode of the expression. Currently used as either a
//	 		 BREAK or CONTINUE expression.
func trueJmpExpressions(prgrm *ast.CXProgram, opcode int) []ast.CXExpression {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0625.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0626.
	expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[opcode])
	expressionIdx := expr.Index

	trueArg := WritePrimary(prgrm, types.BOOL, encoder.Serialize(true), false)
	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, trueArg)
	prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)
	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
	// Now a slice of objects, site S0628.
	return []ast.CXExpression{*exprCXLine, *expr}
}

// BreakExpressions makes a true jump expression for BREAKS.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
func BreakExpressions(prgrm *ast.CXProgram) []ast.CXExpression {
	exprs := trueJmpExpressions(prgrm, constants.OP_BREAK)
	return exprs
}

// ContinueExpressions makes a true jump expression for CONTINUES.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
func ContinueExpressions(prgrm *ast.CXProgram) []ast.CXExpression {
	exprs := trueJmpExpressions(prgrm, constants.OP_CONTINUE)
	return exprs
}

// SelectionExpressions creates series of expressions that will create
// an if else condition.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 conditionExprs - contains the condition of the selection expression.
//		thenExprs - contains the statements if condition is true.
//	 elseExprs - contains the statements if condition is false.
func SelectionExpressions(prgrm *ast.CXProgram, conditionExprs []ast.CXExpression, thenExprs []ast.CXExpression, elseExprs []ast.CXExpression) []ast.CXExpression {
	DefineNewScope(thenExprs)
	DefineNewScope(elseExprs)

	jmpFn := ast.Natives[constants.OP_JMP]
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0629.
	ifExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0630.
	ifExpr := ast.MakeAtomicOperatorExpression(prgrm, jmpFn)
	ifExpressionIdx := ifExpr.Index
	prgrm.CXAtomicOps[ifExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	lastCondExpressionIdx := conditionExprs[len(conditionExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0631.
	lastCondExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastCondExpressionIdx].Operator)

	var predicateTypeSigIdx ast.CXTypeSignatureIndex
	if lastCondExpressionOperator == nil {
		// then it's a literal
		predicateTypeSigIdx = prgrm.CXAtomicOps[lastCondExpressionIdx].GetOutputs(prgrm)[0]
	} else {
		// then it's an expression
		_, predicate := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
		lastCondExpressionOperatorOutputs := lastCondExpressionOperator.GetOutputs(prgrm)
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0633.
		lastCondExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastCondExpressionOperatorOutputs[0])

		var lastCondExpressionOperatorOutputArgType types.Code
		if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0634.
			lastCondExpressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastCondExpressionOperatorOutputTypeSig.Meta))
			lastCondExpressionOperatorOutputArgType = lastCondExpressionOperatorOutputArg.Type
		} else if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
			lastCondExpressionOperatorOutputArgType = types.Code(lastCondExpressionOperatorOutputTypeSig.Meta)
		} else if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			lastCondExpressionOperatorOutputArgType = types.Code(lastCondExpressionOperatorOutputTypeSig.Meta)
		} else {
			panic("type is not known")
		}
		predicate.SetType(lastCondExpressionOperatorOutputArgType)
		predicate.PreviouslyDeclared = true
		predicate.Package = ast.CXPackageIndex(pkg.Index)

		predicateTypeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, predicate)
		prgrm.CXAtomicOps[lastCondExpressionIdx].AddOutput(prgrm, predicateTypeSigIdx)
	}
	prgrm.CXAtomicOps[ifExpressionIdx].AddInput(prgrm, predicateTypeSigIdx)

	thenLines := 0
	// + 1 for cx line expression addition
	elseLines := len(thenExprs) + 1 + 1

	prgrm.CXAtomicOps[ifExpressionIdx].ThenLines = thenLines
	prgrm.CXAtomicOps[ifExpressionIdx].ElseLines = elseLines

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0636.
	skipExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0637.
	skipExpr := ast.MakeAtomicOperatorExpression(prgrm, jmpFn)
	skipExpressionIdx := skipExpr.Index

	prgrm.CXAtomicOps[skipExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	skipLines := len(elseExprs)

	trueArg := WritePrimary(prgrm, types.BOOL, encoder.Serialize(true), false)
	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, trueArg)
	prgrm.CXAtomicOps[skipExpressionIdx].AddInput(prgrm, typeSigIdx)
	prgrm.CXAtomicOps[skipExpressionIdx].ThenLines = skipLines
	prgrm.CXAtomicOps[skipExpressionIdx].ElseLines = 0

	var exprs []ast.CXExpression
	if lastCondExpressionOperator != nil {
		exprs = append(exprs, conditionExprs...)
	} else {
		exprs = append(exprs, conditionExprs[:len(conditionExprs)-1]...)
	}

	exprs = append(exprs, *ifExprCXLine, *ifExpr)
	exprs = append(exprs, thenExprs...)
	exprs = append(exprs, *skipExprCXLine, *skipExpr)
	exprs = append(exprs, elseExprs...)

	return exprs
}

// resolveTypeForUnd tries to determine the type that will be returned from an expression
func resolveTypeForUnd(prgrm *ast.CXProgram, expr *ast.CXExpression) types.Code {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0639.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0640.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	expressionInputs := expression.GetInputs(prgrm)
	expressionOutputs := expression.GetOutputs(prgrm)
	if len(expressionInputs) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0641.
		expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionInputs[0])

		var expressionInputArgType types.Code
		if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0642.
			expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
			expressionInputArgType = expressionInputArg.Type
		} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
			expressionInputArgType = types.Code(expressionInputTypeSig.Meta)
		} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			expressionInputArgType = types.Code(expressionInputTypeSig.Meta)
		} else {
			panic("type is not known")
		}

		// it's a literal
		return expressionInputArgType
	}

	if len(expressionOutputs) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0643.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOutputs[0])
		var expressionOutputArgType types.Code
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0644.
			expressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
			expressionOutputArgType = expressionOutputArg.Type
		} else if expressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			expressionOutputArgType = types.Code(expressionOutputTypeSig.Meta)
		} else if expressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			expressionOutputArgType = types.Code(expressionOutputTypeSig.Meta)
		} else {
			panic("type is not known")
		}

		// it's an expression with an output
		return expressionOutputArgType
	}

	if expressionOperator == nil {
		// the expression doesn't return anything
		return -1
	}
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)
	if len(expressionOperatorOutputs) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0645.
		expressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOperatorOutputs[0])

		var expressionOperatorOutputArgType types.Code
		if expressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0646.
			expressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOperatorOutputTypeSig.Meta))
			expressionOperatorOutputArgType = expressionOperatorOutputArg.Type
		} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
			expressionOperatorOutputArgType = types.Code(expressionOperatorOutputTypeSig.Meta)
		} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			expressionOperatorOutputArgType = types.Code(expressionOperatorOutputTypeSig.Meta)
		} else {
			panic("type is not known")
		}

		// always return first output's type
		return expressionOperatorOutputArgType
	}

	// error
	return -1
}

// IsTempVar ...
// TODO: Delete this function; only called by next function
func IsTempVar(prgrm *ast.CXProgram, nameGid stringpool.StringGid) bool {
	name := prgrm.CXProgramNameText(nameGid)
	if len(name) >= len(constants.LOCAL_PREFIX) && name[:len(constants.LOCAL_PREFIX)] == constants.LOCAL_PREFIX {
		return true
	}
	return false
}

// OperatorExpression creates an expression for an expression OP expression instance.
// i.e. ((expr+expr) * (expr-expr))
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 leftExprs - exprs on the left side of the operator expression
//		rightExprs - exprs on the right side of the operator expression
//		opcode - the opcode of the operator.
func OperatorExpression(prgrm *ast.CXProgram, leftExprs []ast.CXExpression, rightExprs []ast.CXExpression, opcode int) (out []ast.CXExpression) {
	// An empty side failed to compile and its error is recorded.
	if len(leftExprs) == 0 || len(rightExprs) == 0 {
		return nil
	}
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	lastLeftExpressionIdx := leftExprs[len(leftExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0647.
	lastLeftExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastLeftExpressionIdx].Operator)
	if len(prgrm.CXAtomicOps[lastLeftExpressionIdx].GetOutputs(prgrm)) < 1 {
		operandResultOutputAdd(prgrm, pkg, leftExprs, lastLeftExpressionOperator)
	}

	lastRightExpressionIdx := rightExprs[len(rightExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0658.
	lastRightExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastRightExpressionIdx].Operator)
	if len(prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)) < 1 {
		operandResultOutputAdd(prgrm, pkg, rightExprs, lastRightExpressionOperator)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0670.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0671.
	expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[opcode])
	expressionIdx := expr.Index

	// we can't know the type until we compile the full function
	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0672.
	lastLeftExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastLeftExpressionIdx].GetOutputs(prgrm)[0])
	switch lastLeftExpressionOutputTypeSig.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE, ast.TYPE_ATOMIC, ast.TYPE_POINTER_ATOMIC, ast.TYPE_SLICE_ATOMIC:
	default:
		panic("type is not known")
	}

	if operandIndexed(prgrm, lastLeftExpressionOutputTypeSig) || lastLeftExpressionOperator != nil {
		typeSigIdx := prgrm.CXAtomicOps[lastLeftExpressionIdx].GetOutputs(prgrm)[0]
		// then it's a function call or an array access
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)

		// The placeholder of f()[i] has no operator and is dropped like a variable.
		if lastLeftExpressionOperator != nil && IsTempVar(prgrm, lastLeftExpressionOutputTypeSig.Name) {
			out = append(out, leftExprs...)
		} else {
			out = append(out, leftExprs[:len(leftExprs)-1]...)
		}
	} else {
		typeSigIdx := prgrm.CXAtomicOps[lastLeftExpressionIdx].GetOutputs(prgrm)[0]
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)
		// A variable read after a method call keeps the call.
		out = append(out, leftExprs[:len(leftExprs)-1]...)
	}
	leftCount := len(out)

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0676.
	lastRightExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)[0])
	// nil for an atomic output.
	var lastRightExpressionOutputArg *ast.CXArgument
	if lastRightExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0678.
		lastRightExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastRightExpressionOutputTypeSig.Meta))
	} else if lastRightExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || lastRightExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
		// An atomic output has no indexes.
	} else if lastRightExpressionOutputTypeSig.Type != ast.TYPE_SLICE_ATOMIC {
		panic("type is not known")
	}

	if (opcode == constants.OP_BOOL_AND || opcode == constants.OP_BOOL_OR) && lastRightExpressionOperator == nil && (operandIndexed(prgrm, lastRightExpressionOutputTypeSig) || lastRightExpressionOutputArg != nil && len(lastRightExpressionOutputArg.DereferenceOperations) > 0) {
		lastRightExpressionOperator = shortCircuitOperandExpression(prgrm, lastRightExpressionIdx)
		lastRightExpressionOutputTypeSig = prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)[0])
	}

	if operandIndexed(prgrm, lastRightExpressionOutputTypeSig) || lastRightExpressionOperator != nil {
		typeSigIdx := prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)[0]
		// then it's a function call or an array access
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)

		if lastRightExpressionOperator != nil && IsTempVar(prgrm, lastRightExpressionOutputTypeSig.Name) {
			out = append(out, rightExprs...)
		} else {
			out = append(out, rightExprs[:len(rightExprs)-1]...)
		}
	} else {
		typeSigIdx := prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)[0]
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)
		out = append(out, rightExprs[:len(rightExprs)-1]...)
	}

	if opcode == constants.OP_BOOL_AND || opcode == constants.OP_BOOL_OR {
		out = shortCircuitExpressions(prgrm, out, leftCount, prgrm.CXAtomicOps[expressionIdx].GetInputs(prgrm)[0], opcode)
	}
	out = append(out, *exprCXLine, *expr)

	return
}

// operandResultOutputAdd gives the last expression of exprs, a call or an
// operator with no output yet, a temporary output of the type it returns.
func operandResultOutputAdd(prgrm *ast.CXProgram, pkg *ast.CXPackage, exprs []ast.CXExpression, operator *ast.CXFunction) {
	exprIdx := exprs[len(exprs)-1].Index
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0648.
	resultTypeSig := prgrm.GetCXTypeSignatureFromArray(operator.GetOutputs(prgrm)[0])

	var typeSigIdx ast.CXTypeSignatureIndex
	if resultTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0649.
		resultArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(resultTypeSig.Meta))

		_, out := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
		out.SetType(resolveTypeForUnd(prgrm, &exprs[len(exprs)-1]))
		out.Size = resultArg.Size
		out.Type = resultArg.Type
		out.PointerTargetType = resultArg.PointerTargetType
		out.Package = ast.CXPackageIndex(pkg.Index)
		out.PreviouslyDeclared = true

		typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
	} else if resultTypeSig.Type == ast.TYPE_ATOMIC {
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
		_, typeSig := prgrm.AllocCXTypeSignature()
		typeSig.ArgDetails = argDetailsGid
		typeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
		typeSig.Package = resultTypeSig.Package
		typeSig.Type = ast.TYPE_ATOMIC
		typeSig.Meta = resultTypeSig.Meta
		typeSig.Offset = resultTypeSig.Offset

		typeSigIdx = typeSig.Index
	} else if resultTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
		_, typeSig := prgrm.AllocCXTypeSignature()
		typeSig.ArgDetails = argDetailsGid
		typeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
		typeSig.Package = resultTypeSig.Package
		typeSig.Type = ast.TYPE_POINTER_ATOMIC
		typeSig.Meta = resultTypeSig.Meta
		typeSig.Offset = resultTypeSig.Offset

		typeSigIdx = typeSig.Index
	} else {
		panic("type is not known")
	}

	prgrm.CXAtomicOps[exprIdx].AddOutput(prgrm, typeSigIdx)
}

// operandIndexed reports whether the operand sig reads an element, as a[i]
// or the slice placeholder of f()[i] do.
func operandIndexed(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) bool {
	switch sig.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		return len(prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta)).Indexes) > 0
	case ast.TYPE_SLICE_ATOMIC:
		return len(prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta).Indexes) > 0
	}
	return false
}

// shortCircuitLinesPending marks the skip of a short-circuit jmp until ProcessShortCircuitJumps sets it.
const shortCircuitLinesPending = -1 << 30

// shortCircuitExpressions puts a jmp on the left operand before the right operand's expressions.
// The bool op still runs, and when the right side is skipped the left operand alone decides it.
func shortCircuitExpressions(prgrm *ast.CXProgram, exprs []ast.CXExpression, leftCount int, leftTypeSigIdx ast.CXTypeSignatureIndex, opcode int) []ast.CXExpression {
	if leftCount == len(exprs) {
		return exprs
	}
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	jmpCXLine := MakeCXLineExpressionCurrent(prgrm)
	jmpExpr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_JMP])
	jmpOp := &prgrm.CXAtomicOps[jmpExpr.Index]
	jmpOp.Package = ast.CXPackageIndex(pkg.Index)
	jmpOp.AddInput(prgrm, leftTypeSigIdx)
	if opcode == constants.OP_BOOL_OR {
		jmpOp.ThenLines = shortCircuitLinesPending
	} else {
		jmpOp.ElseLines = shortCircuitLinesPending
	}

	out := make([]ast.CXExpression, 0, len(exprs)+2)
	out = append(out, exprs[:leftCount]...)
	out = append(out, *jmpCXLine, *jmpExpr)
	return append(out, exprs[leftCount:]...)
}

// shortCircuitOperandExpression copies an indexed or dereferenced right operand into a bool temp,
// so the index or pointer is read only when the right side runs.
func shortCircuitOperandExpression(prgrm *ast.CXProgram, exprIdx int) *ast.CXFunction {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	operandOp := &prgrm.CXAtomicOps[exprIdx]
	operandOp.Inputs = nil
	operandOp.AddInput(prgrm, operandOp.GetOutputs(prgrm)[0])
	operandOp.Outputs = nil

	_, temp := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
	temp.SetType(types.BOOL)
	temp.PreviouslyDeclared = true
	temp.Package = ast.CXPackageIndex(pkg.Index)
	operandOp.AddOutput(prgrm, ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, temp))

	operandOp.ThenLines = shortCircuitLinesPending
	operandOp.ElseLines = shortCircuitLinesPending
	identityIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_IDENTITY])
	operandOp.Operator = identityIdx
	return prgrm.GetFunctionFromArray(identityIdx)
}

// ShortCircuitJumpCheck reports whether exprs[currIndex] is a short-circuit jmp. Its bool op
// reads the same left operand and reports a type error on it, so the jmp is not type checked.
func ShortCircuitJumpCheck(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) bool {
	jmpOp := &prgrm.CXAtomicOps[exprs[currIndex].Index]
	jmpOperator := prgrm.GetFunctionFromArray(jmpOp.Operator)
	if jmpOperator == nil || jmpOperator.AtomicOPCode != constants.OP_JMP || len(jmpOp.GetInputs(prgrm)) != 1 {
		return false
	}
	boolOpcode := constants.OP_BOOL_OR
	target := currIndex + jmpOp.ThenLines + 1
	if jmpOp.ThenLines == 0 {
		boolOpcode = constants.OP_BOOL_AND
		target = currIndex + jmpOp.ElseLines + 1
	}
	if target <= currIndex+1 || target >= len(exprs) || exprs[target].Type != ast.CX_ATOMIC_OPERATOR {
		return false
	}
	boolOp := &prgrm.CXAtomicOps[exprs[target].Index]
	boolOperator := prgrm.GetFunctionFromArray(boolOp.Operator)
	return boolOperator != nil && boolOperator.AtomicOPCode == boolOpcode && len(boolOp.GetInputs(prgrm)) > 0 && boolOp.GetInputs(prgrm)[0] == jmpOp.GetInputs(prgrm)[0]
}

// ProcessShortCircuitJumps points each short-circuit jmp at its bool op, once the function's expressions are final.
// A jmp whose bool op is not found skips nothing, so both operands run.
func ProcessShortCircuitJumps(prgrm *ast.CXProgram, exprs []ast.CXExpression) {
	for i := range exprs {
		if exprs[i].Type != ast.CX_ATOMIC_OPERATOR {
			continue
		}
		jmpOp := &prgrm.CXAtomicOps[exprs[i].Index]
		if jmpOp.ThenLines != shortCircuitLinesPending && jmpOp.ElseLines != shortCircuitLinesPending {
			continue
		}
		if jmpOp.ThenLines == shortCircuitLinesPending && jmpOp.ElseLines == shortCircuitLinesPending {
			// An operand copy from shortCircuitOperandExpression.
			jmpOp.ThenLines = 0
			jmpOp.ElseLines = 0
			continue
		}
		boolOpcode := constants.OP_BOOL_AND
		if jmpOp.ThenLines == shortCircuitLinesPending {
			boolOpcode = constants.OP_BOOL_OR
		}
		jmpOp.ThenLines = 0
		jmpOp.ElseLines = 0
		leftTypeSigIdx := jmpOp.GetInputs(prgrm)[0]
		for j := i + 1; j < len(exprs); j++ {
			if exprs[j].Type != ast.CX_ATOMIC_OPERATOR {
				continue
			}
			boolOp := &prgrm.CXAtomicOps[exprs[j].Index]
			boolOperator := prgrm.GetFunctionFromArray(boolOp.Operator)
			if boolOperator == nil || boolOperator.AtomicOPCode != boolOpcode || len(boolOp.GetInputs(prgrm)) == 0 || boolOp.GetInputs(prgrm)[0] != leftTypeSigIdx {
				continue
			}
			if boolOpcode == constants.OP_BOOL_OR {
				jmpOp.ThenLines = j - i - 1
			} else {
				jmpOp.ElseLines = j - i - 1
			}
			break
		}
	}
}

// UnaryExpression creates an expression for unary operations,
// operations that only need one input, '*', '&', '!', and '-'.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 op - the unary operator, '*', '&', '!', and '-'.
//	 prevExprs - the array of expressions the unary expression belongs.
func UnaryExpression(prgrm *ast.CXProgram, op string, prevExprs []ast.CXExpression) []ast.CXExpression {
	if len(prevExprs) == 0 {
		return nil
	}
	lastPrevExpression, err := prgrm.GetCXAtomicOpFromExpressions(prevExprs, len(prevExprs)-1)
	if err != nil {
		panic(err)
	}

	if op == "-" || op == "!" {
		if exprs, ok := unaryOperatorOnValue(prgrm, op, prevExprs); ok {
			return exprs
		}
	}

	if len(lastPrevExpression.GetOutputs(prgrm)) == 0 {
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeInvalidIndirection, CurrentFile, LineNo, ast.CompileErrorOperands{})
		// A stand in typed types.INVALID calls the operand, so its uses report nothing more.
		lastPrevExpression.Operator = invalidFunctionIndex(prgrm, lastPrevExpression.Package, prgrm.CXProgramNameIntern(op))
		return prevExprs
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0680.
	lastPrevExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastPrevExpression.GetOutputs(prgrm)[0])
	var lastPrevExpressionOutputArg *ast.CXArgument
	if lastPrevExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0682.
		lastPrevExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastPrevExpressionOutputTypeSig.Meta))

		// Some properties need to be read from the base argument
		// due to how we calculate dereferences at the moment.
		lastPrevExpressionOutput := lastPrevExpressionOutputArg
		baseOut := lastPrevExpressionOutput
		exprOut := lastPrevExpressionOutput.GetAssignmentElement(prgrm)
		switch op {
		case "*":
			exprOut.DereferenceOperations = append(exprOut.DereferenceOperations, constants.DEREF_POINTER)
			exprOut.DeclarationSpecifiers = append(exprOut.DeclarationSpecifiers, constants.DECL_DEREF)
		case "&":
			baseOut.PassBy = constants.PASSBY_REFERENCE

			exprOut.DeclarationSpecifiers = append(exprOut.DeclarationSpecifiers, constants.DECL_POINTER)
		case "!":
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now a constructor call, site S0683.
				expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BOOL_NOT])
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0684.
				expression, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.Package = ast.CXPackageIndex(pkg.Index)

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0685.
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArgFromArray(ast.CXArgumentIndex(exprOut.Index)))
				expression.AddInput(prgrm, typeSigIdx)
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now an object copy, site S0687.
				prevExprs[len(prevExprs)-1] = *expr
			} else {
				panic(err)
			}
		case "-":
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now a constructor call, site S0688.
				expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_NEG])
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0689.
				expression, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.Package = ast.CXPackageIndex(pkg.Index)
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0690.
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArgFromArray(ast.CXArgumentIndex(exprOut.Index)))
				expression.AddInput(prgrm, typeSigIdx)
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now an object copy, site S0692.
				prevExprs[len(prevExprs)-1] = *expr
			} else {
				panic(err)
			}
		}
	} else if lastPrevExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
		switch op {
		case "*":
			// TODO: what is the most efficient alternative for this
			// exprOut.DereferenceOperations = append(exprOut.DereferenceOperations, constants.DEREF_POINTER)
			// exprOut.DeclarationSpecifiers = append(exprOut.DeclarationSpecifiers, constants.DECL_DEREF)

			// a pointer type that we want to get its value
			lastPrevExpressionOutputTypeSig.IsDeref = true
		case "&":
			lastPrevExpressionOutputTypeSig.PassBy = constants.PASSBY_REFERENCE
		case "!":
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now a constructor call, site S0693.
				expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BOOL_NOT])
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0694.
				expression, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.Package = ast.CXPackageIndex(pkg.Index)

				typeSigIdx := lastPrevExpressionOutputTypeSig.Index
				expression.AddInput(prgrm, typeSigIdx)
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now an object copy, site S0695.
				prevExprs[len(prevExprs)-1] = *expr
			} else {
				panic(err)
			}
		case "-":
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now a constructor call, site S0696.
				expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_NEG])
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0697.
				expression, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.Package = ast.CXPackageIndex(pkg.Index)

				typeSigIdx := lastPrevExpressionOutputTypeSig.Index
				expression.AddInput(prgrm, typeSigIdx)
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now an object copy, site S0698.
				prevExprs[len(prevExprs)-1] = *expr
			} else {
				panic(err)
			}
		}

	} else {
		panic("type is not cx argument deprecate\n\n")
	}

	return prevExprs
}

// AssociateReturnExpressions associate the output of `retExprs` to the
// `idx`th output parameter of the current function.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 idx - referring to the nth output parameter of the current function.
//		retExprs - the return expressions to be associated or linked to the
//				   output parameter of the current function.
func AssociateReturnExpressions(prgrm *ast.CXProgram, idx int, retExprs []ast.CXExpression) []ast.CXExpression {
	var pkg *ast.CXPackage
	var fn *ast.CXFunction
	var err error

	pkg, err = prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	fn, err = pkg.GetCurrentFunction(prgrm)
	if err != nil {
		panic(err)
	}

	fnOutputs := fn.GetOutputs(prgrm)
	// AddJmpToReturnExpressions reports a return count that does not match.
	if idx >= len(fnOutputs) || len(retExprs) == 0 {
		return retExprs
	}
	lastExpr := retExprs[len(retExprs)-1]

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0699.
	fnOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(fnOutputs[idx])
	var outParam *ast.CXArgument
	// pointerArrayOut is the result of a *[N]T function, which return &a
	// writes the address to, as r = &a does.
	var pointerArrayOut *ast.CXArgument
	var typeSigIdx ast.CXTypeSignatureIndex
	if fnOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0701.
		outParam = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fnOutputTypeSig.Meta))

		_, out := ast.MakeArgument(prgrm, outParam.Name, CurrentFile, LineNo)
		out.SetType(outParam.Type)
		out.StructType = outParam.StructType
		out.PreviouslyDeclared = true

		if ast.IsTypeArrayAtomic(outParam) {
			// return [2]i32{1, 2} checks the literal against the whole result type.
			out.Lengths = slices.Clone(outParam.Lengths)
			out.DeclarationSpecifiers = slices.Clone(outParam.DeclarationSpecifiers)
			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, out)
		} else if ast.IsTypePointerArrayAtomic(outParam) {
			// The result takes its pointer specifiers from its declaration.
			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, out)
			pointerArrayOut = out
		} else {
			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
		}
	} else if fnOutputTypeSig.Type == ast.TYPE_ATOMIC {
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
		_, newCXTypeSig := prgrm.AllocCXTypeSignature()
		newCXTypeSig.ArgDetails = argDetailsGid
		newCXTypeSig.Name = fnOutputTypeSig.Name
		newCXTypeSig.Package = fnOutputTypeSig.Package
		newCXTypeSig.Type = ast.TYPE_ATOMIC
		newCXTypeSig.Meta = int(fnOutputTypeSig.Type)
		newCXTypeSig.Offset = fnOutputTypeSig.Offset
		typeSigIdx = newCXTypeSig.Index
	} else if fnOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
		_, newCXTypeSig := prgrm.AllocCXTypeSignature()
		newCXTypeSig.ArgDetails = argDetailsGid
		newCXTypeSig.Name = fnOutputTypeSig.Name
		newCXTypeSig.Package = fnOutputTypeSig.Package
		newCXTypeSig.Type = ast.TYPE_POINTER_ATOMIC
		newCXTypeSig.Meta = int(fnOutputTypeSig.Type)
		newCXTypeSig.Offset = fnOutputTypeSig.Offset
		typeSigIdx = newCXTypeSig.Index
	} else if fnOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
		_, newCXTypeSig := prgrm.AllocCXTypeSignature()
		newCXTypeSig.ArgDetails = argDetailsGid
		newCXTypeSig.Name = fnOutputTypeSig.Name
		newCXTypeSig.Package = fnOutputTypeSig.Package
		newCXTypeSig.Type = ast.TYPE_SLICE_ATOMIC
		newCXTypeSig.Meta = fnOutputTypeSig.Meta
		newCXTypeSig.Offset = fnOutputTypeSig.Offset
		typeSigIdx = newCXTypeSig.Index
	} else if fnOutputTypeSig.Type == ast.TYPE_STRUCT || fnOutputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC || fnOutputTypeSig.Type == ast.TYPE_POINTER_STRUCT || fnOutputTypeSig.Type == ast.TYPE_POINTER_ARRAY_ATOMIC || fnOutputTypeSig.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
		// A struct, array or pointer output, as in return T{x: 1} or return &T{}.
		_, newCXTypeSig := prgrm.AllocCXTypeSignatureCopy(fnOutputTypeSig)
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
		newCXTypeSig.ArgDetails = argDetailsGid
		typeSigIdx = newCXTypeSig.Index
	} else {
		panic("type is not known")
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0713.
	lastExpression, err := prgrm.GetCXAtomicOp(lastExpr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0714.
	lastExpressionOperator := prgrm.GetFunctionFromArray(lastExpression.Operator)
	if lastExpressionOperator == nil {
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now an arena insert, site S0715.
		opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_IDENTITY])
		lastExpression.Operator = opIdx

		lastExpression.Inputs = lastExpression.Outputs
		lastExpression.Outputs = nil

		if inputs := lastExpression.GetInputs(prgrm); pointerArrayOut != nil && len(inputs) > 0 && ast.CXTypeSignaturePassBy(prgrm, prgrm.GetCXTypeSignatureFromArray(inputs[0])) == constants.PASSBY_REFERENCE {
			pointerArrayOut.PassBy = constants.PASSBY_REFERENCE
			prgrm.GetCXTypeSignatureFromArray(typeSigIdx).PassBy = constants.PASSBY_REFERENCE
		}
		lastExpression.AddOutput(prgrm, typeSigIdx)
		if len(retExprs) < 2 || retExprs[len(retExprs)-2].Type != ast.CX_LINE {
			// return s[0][1] has no line of its own for a runtime error to report.
			lineExprs := append([]ast.CXExpression{}, retExprs[:len(retExprs)-1]...)
			return append(lineExprs, *MakeCXLineExpressionCurrent(prgrm), lastExpr)
		}
		return retExprs
	} else if lastExpr.IsArrayLiteral() && len(lastExpression.GetOutputs(prgrm)) > 0 {
		// return [2]i32{1, 2} writes the literal to the result.
		lastExpression.Outputs = nil
		lastExpression.AddOutput(prgrm, typeSigIdx)
		retExprs[len(retExprs)-1].ExpressionType = ast.CXEXPR_UNUSED
		return retExprs
	} else if len(lastExpression.GetOutputs(prgrm)) > 0 {
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0716.
		exprCXLine := MakeCXLineExpressionCurrent(prgrm)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0717.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0718.
		expression, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		exprOutTypeSig := lastExpression.GetOutputs(prgrm)[0]
		expression.AddInput(prgrm, exprOutTypeSig)
		expression.AddOutput(prgrm, typeSigIdx)

		return append(retExprs, *exprCXLine, *expr)
	} else {
		lastExpression.AddOutput(prgrm, typeSigIdx)
		return retExprs
	}
}

// AddJmpToReturnExpressions adds an jump expression that makes a function stop its execution.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 exprs - contains the array of return expressions where the jump expr will be added.
func AddJmpToReturnExpressions(prgrm *ast.CXProgram, exprs ReturnExpressions) []ast.CXExpression {
	var pkg *ast.CXPackage
	var fn *ast.CXFunction
	var err error

	pkg, err = prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	fn, err = pkg.GetCurrentFunction(prgrm)
	if err != nil {
		panic(err)
	}

	retExprs := exprs.Expressions
	fnOutputs := fn.GetOutputs(prgrm)
	if len(fnOutputs) != exprs.Size && exprs.Expressions != nil {
		operands := ast.CompileErrorOperands{int64(fn.Name), int64(len(fnOutputs)), int64(exprs.Size)}
		if exprs.Column > 0 {
			prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeFunctionReturnCount, CurrentFile, exprs.Line, exprs.Column, operands)
		} else {
			file, line := CurrentFile, LineNo
			if lastExprCXLine, err := prgrm.GetPreviousCXLine(retExprs, len(retExprs)-1); err == nil {
				file, line = lastExprCXLine.FileName, lastExprCXLine.LineNumber
			}
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeFunctionReturnCount, file, line, operands)
		}
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0719.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// expression to jump to the end of the embedding function
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0720.
	expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_GOTO])
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0721.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// simulating a label so it gets executed without evaluating a predicate
	expression.Label = generateTempVarName(prgrm, constants.LABEL_PREFIX)
	expression.ThenLines = types.MAX_INT32
	expression.Package = ast.CXPackageIndex(pkg.Index)

	retExprs = append(retExprs, *exprCXLine, *expr)

	return retExprs
}

// ShortCircuitStrip turns the && and || of a struct literal field back into eager bool ops.
// A struct literal takes every expression of a field value as the field itself.
func ShortCircuitStrip(prgrm *ast.CXProgram, exprs []ast.CXExpression) []ast.CXExpression {
	out := make([]ast.CXExpression, 0, len(exprs))
	for i := 0; i < len(exprs); i++ {
		if exprs[i].Type != ast.CX_ATOMIC_OPERATOR {
			out = append(out, exprs[i])
			continue
		}
		op := &prgrm.CXAtomicOps[exprs[i].Index]
		thenPending := op.ThenLines == shortCircuitLinesPending
		elsePending := op.ElseLines == shortCircuitLinesPending
		if !thenPending && !elsePending {
			out = append(out, exprs[i])
			continue
		}
		op.ThenLines = 0
		op.ElseLines = 0
		if thenPending && elsePending {
			copyOutput := op.GetOutputs(prgrm)[0]
			for j := i + 1; j < len(exprs); j++ {
				if exprs[j].Type != ast.CX_ATOMIC_OPERATOR {
					continue
				}
				boolInputs := prgrm.CXAtomicOps[exprs[j].Index].GetInputs(prgrm)
				if len(boolInputs) == 2 && boolInputs[1] == copyOutput {
					boolInputs[1] = op.GetInputs(prgrm)[0]
					break
				}
			}
			continue
		}
		if len(out) > 0 && out[len(out)-1].Type == ast.CX_LINE {
			out = out[:len(out)-1]
		}
	}
	return out
}

// unaryOperatorOnValue applies - or ! to a call, an operator result or a
// struct field read. It reports false for the operands UnaryExpression handles.
func unaryOperatorOnValue(prgrm *ast.CXProgram, op string, prevExprs []ast.CXExpression) ([]ast.CXExpression, bool) {
	lastIdx := prevExprs[len(prevExprs)-1].Index
	operator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastIdx].Operator)
	operand := prevExprs
	if operator != nil {
		if len(prgrm.CXAtomicOps[lastIdx].GetOutputs(prgrm)) == 0 {
			if len(operator.GetOutputs(prgrm)) == 0 {
				return nil, false
			}
			pkg, err := prgrm.GetCurrentPackage()
			if err != nil {
				panic(err)
			}
			operandResultOutputAdd(prgrm, pkg, prevExprs, operator)
		}
	} else {
		outputs := prgrm.CXAtomicOps[lastIdx].GetOutputs(prgrm)
		if len(outputs) == 0 {
			return nil, false
		}
		sig := prgrm.GetCXTypeSignatureFromArray(outputs[0])
		if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE || len(prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta)).Fields) == 0 {
			return nil, false
		}
		// The read is replaced, and the expressions before it still run.
		operand = prevExprs[:len(prevExprs)-1]
	}

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	opcode := constants.OP_NEG
	if op == "!" {
		opcode = constants.OP_BOOL_NOT
	}
	expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[opcode])
	prgrm.CXAtomicOps[expr.Index].Package = ast.CXPackageIndex(pkg.Index)
	prgrm.CXAtomicOps[expr.Index].AddInput(prgrm, prgrm.CXAtomicOps[lastIdx].GetOutputs(prgrm)[0])
	return append(operand, *expr), true
}

// invalidPlaceholderTypeSig returns a temporary of types.INVALID, the value
// of an expression whose compile error is recorded.
func invalidPlaceholderTypeSig(prgrm *ast.CXProgram, pkgIdx ast.CXPackageIndex) ast.CXTypeSignatureIndex {
	_, placeholder := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
	placeholder.SetType(types.INVALID)
	placeholder.Package = pkgIdx
	placeholder.PreviouslyDeclared = true
	return ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, placeholder)
}
