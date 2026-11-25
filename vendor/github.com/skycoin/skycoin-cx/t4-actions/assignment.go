package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// assignStructLiteralFields converts a struct literal to a series of struct field assignments.
// For example, `foo = Item{x: 10, y: 20}` is converted to: `foo.x = 10; foo.y = 20;`.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	toExprs - toExprs are the array of expressions that contains the data needed
//			  to construct the series of struct field assignments.
//	fromExprs - fromExprs are the array of expressions that will contain the
//			    series of struct field assignments.
//	structLiteralName - name of the struct, in the example above this is "foo".
func assignStructLiteralFields(prgrm *ast.CXProgram, toExprs []ast.CXExpression, fromExprs []ast.CXExpression, structLiteralName stringpool.StringGid) []ast.CXExpression {
	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	for _, exprInfo := range fromExprs {
		if exprInfo.Type == ast.CX_LINE || exprInfo.IsMethodCall() {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0512.
		expression, err := prgrm.GetCXAtomicOp(exprInfo.Index)
		if err != nil {
			panic(err)
		}

		expressionOutputTypeSigIdx := expression.GetOutputs(prgrm)[0]
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0513.
		expressionOutput := prgrm.GetCXTypeSignatureFromArray(expressionOutputTypeSigIdx)

		if expressionOutput.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			expressionOutputIdx := ast.CXArgumentIndex(expressionOutput.Meta)

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1634.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Name = structLiteralName
			expressionOutput.Name = structLiteralName

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0514.
			toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])
			// nil for an atomic output.
			var toExpressionOutput *ast.CXArgument
			if toExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0516.
				toExpressionOutput = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(toExpressionOutputTypeSig.Meta))
			} else if toExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || toExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				// TODO: Improve when possible
				// Do nothing for now since len(toExpressionOutput.Indexes) > 0
				// of type atomics and type pointer atomics are always 0
			} else {
				panic("type is not known")
			}

			if toExpressionOutput != nil && len(toExpressionOutput.Indexes) > 0 {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1635.
				prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Lengths = toExpressionOutput.Lengths
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1636.
				prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Indexes = toExpressionOutput.Indexes
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1637.
				prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations = append(prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations, constants.DEREF_ARRAY)
			}

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1638.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations = append(prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations, constants.DEREF_FIELD)
		} else {
			panic("type is not type cx argument deprecate\n\n")
		}
	}

	return fromExprs
}

// StructLiteralAssignment handles struct literals, e.g. `Item{x: 10, y: 20}`, and references to
// struct literals, e.g. `&Item{x: 10, y: 20}` in assignment expressions.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	toExprs - toExprs are the array of expressions that contains the data needed
//			  to construct the series of struct field assignments.
//	fromExprs - fromExprs are the array of expressions that will contain the
//				series of struct field assignments.
func StructLiteralAssignment(prgrm *ast.CXProgram, toExprs []ast.CXExpression, fromExprs []ast.CXExpression) []ast.CXExpression {
	lastFromExpr := fromExprs[len(fromExprs)-1]

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0517.
	lastFromExpression, err := prgrm.GetCXAtomicOp(lastFromExpr.Index)
	if err != nil {
		panic(err)
	}

	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	lastFromCXLine, _ := prgrm.GetPreviousCXLine(fromExprs, len(fromExprs)-1)

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0518.
	lastFromExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastFromExpression.GetOutputs(prgrm)[0])
	var lastFromExpressionOutputArg *ast.CXArgument
	if lastFromExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0520.
		lastFromExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastFromExpressionOutputTypeSig.Meta))
	} else {
		panic("type is not cx argument deprecate\n\n")
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0521.
	toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])

	// If the last expression in `fromExprs` is declared as pointer
	// then it means the whole struct literal needs to be passed by reference.
	if !hasDeclSpec(lastFromExpressionOutputArg.GetAssignmentElement(prgrm), constants.DECL_POINTER) {
		return assignStructLiteralFields(prgrm, toExprs, fromExprs, toExpressionOutputTypeSig.Name)
	} else {
		// And we also need an auxiliary variable to point to,
		// otherwise we'd be trying to assign the fields to a nil value.
		outField := lastFromExpressionOutputArg
		auxName := generateTempVarName(prgrm, constants.LOCAL_PREFIX)
		_, aux := ast.MakeArgument(prgrm, auxName, lastFromCXLine.FileName, lastFromCXLine.LineNumber)
		aux.SetType(outField.Type)
		aux.DeclarationSpecifiers = append(aux.DeclarationSpecifiers, constants.DECL_POINTER)
		aux.StructType = outField.StructType
		aux.Size = outField.Size
		aux.PreviouslyDeclared = true
		aux.Package = lastFromExpression.Package

		auxTypeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, aux)

		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0524.
		declExprCXLine := ast.MakeCXLineExpression(prgrm, lastFromCXLine.FileName, lastFromCXLine.LineNumber, lastFromCXLine.LineStr)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0525.
		declExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
		declExpressionIdx := declExpr.Index

		prgrm.CXAtomicOps[declExpressionIdx].Package = lastFromExpression.Package
		prgrm.CXAtomicOps[declExpressionIdx].AddOutput(prgrm, auxTypeSigIdx)

		_, out := ast.MakeArgument(prgrm, toExpressionOutputTypeSig.Name, lastFromCXLine.FileName, lastFromCXLine.LineNumber)
		out.Package = lastFromExpression.Package
		outTypeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)

		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0528.
		assignExprCXLine := ast.MakeCXLineExpression(prgrm, lastFromCXLine.FileName, lastFromCXLine.LineNumber, lastFromCXLine.LineStr)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0529.
		assignExpr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_IDENTITY))
		assignExpressionIdx := assignExpr.Index

		prgrm.CXAtomicOps[assignExpressionIdx].Package = lastFromExpression.Package
		prgrm.CXAtomicOps[assignExpressionIdx].AddOutput(prgrm, outTypeSigIdx)
		prgrm.CXAtomicOps[assignExpressionIdx].AddInput(prgrm, auxTypeSigIdx)

		fromExprs = assignStructLiteralFields(prgrm, toExprs, fromExprs, auxName)
		// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
		// Now a slice of objects, site S0530.
		fromExprs = append([]ast.CXExpression{*declExprCXLine, *declExpr}, fromExprs...)

		return append(fromExprs, *assignExprCXLine, *assignExpr)
	}
}

// ArrayLiteralAssignment handles array literals.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	toExprs - toExprs are the array of expressions that contains the data needed
//			  to construct the series of array literals.
//	fromExprs - fromExprs are the array of expressions that will contain the
//				series of array literal assignments.
func ArrayLiteralAssignment(prgrm *ast.CXProgram, toExprs []ast.CXExpression, fromExprs []ast.CXExpression) []ast.CXExpression {
	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0531.
	toExpressionOutput := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])

	for _, expr := range fromExprs {
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0532.
		expression, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0533.
		expressionOutput := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		var expressionOutputIdx ast.CXArgumentIndex
		if expressionOutput.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			expressionOutputIdx = ast.CXArgumentIndex(expressionOutput.Meta)
		} else {
			panic("type is not type cx argument deprecate\n\n")
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1639.
		prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Name = toExpressionOutput.Name
		expressionOutput.Name = toExpressionOutput.Name
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1640.
		prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations = append(prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations, constants.DEREF_ARRAY)
	}

	return fromExprs
}

// ShortAssignment handles short assignments for ">>=","<<=",
// "+=","-=","*=","/=","%=","&=","^=", and "|=" operators.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	expr - the expression for the short assignment.
//	exprCXLine - the cx line or the line string of the short assignment expression.
//	toExprs - Contains the output cx arg to be added to the expression.
//	fromExprs - Contains the output cx arg to be added to the expression.
//	pkg - the package the expression belongs.
func ShortAssignment(prgrm *ast.CXProgram, expr *ast.CXExpression, exprCXLine *ast.CXExpression, toExprs []ast.CXExpression, fromExprs []ast.CXExpression, pkg *ast.CXPackage) []ast.CXExpression {
	expressionIdx := expr.Index

	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	fromExpressionIdx := fromExprs[len(fromExprs)-1].Index

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0534.
	fromExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[fromExpressionIdx].Operator)

	typeSig := toExpression.GetOutputs(prgrm)[0]

	prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, ArgumentReadCopy(prgrm, typeSig))
	prgrm.CXAtomicOps[expressionIdx].AddOutput(prgrm, typeSig)

	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	if fromExpressionOperator == nil {
		typeSig := prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)[0]
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSig)
	} else {
		var fromExpressionInputArg *ast.CXArgument
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0536.
		fromExpressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetInputs(prgrm)[0])
		var typeSigIdx ast.CXTypeSignatureIndex
		if fromExpressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0537.
			fromExpressionInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionInputTypeSig.Meta))

			_, sym := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
			sym.SetType(fromExpressionInputArg.Type)
			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, sym)
		} else if fromExpressionInputTypeSig.Type == ast.TYPE_ATOMIC || fromExpressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(fromExpressionInputTypeSig)
			newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			newTypeSig.Package = ast.CXPackageIndex(pkg.Index)
			typeSigIdx = newTypeSig.Index
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[fromExpressionIdx].AddOutput(prgrm, typeSigIdx)
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)
	}

	// A variable read is an input, and the expressions before it still run.
	if fromExpressionOperator == nil && len(prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)) > 0 && len(prgrm.CXAtomicOps[fromExpressionIdx].GetInputs(prgrm)) == 0 {
		// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
		// Now a slice of objects, site S0543.
		return append(fromExprs[:len(fromExprs)-1], *exprCXLine, *expr)
	} else {
		return append(fromExprs, *exprCXLine, *expr)
	}
}

// getOutputType tries to determine the expr's type that should be
// returned by a function call.
// This function is needed because CX has some standard library functions that return cxcore.TYPE_UNDEFINED
// arguments. In these cases, the output type depends on its input arguments' type. In the rest of
// the cases, we can simply use the function's return type.
func getOutputType(prgrm *ast.CXProgram, expr *ast.CXExpression) *ast.CXTypeSignature {
	return getOutputTypeAt(prgrm, expr, 0)
}

// getOutputTypeAt is getOutputType for the output at index of a call with
// several outputs.
func getOutputTypeAt(prgrm *ast.CXProgram, expr *ast.CXExpression, index int) *ast.CXTypeSignature {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0544.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0545.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0546.
	expressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOperatorOutputs[index])
	if expressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0547.
		expressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOperatorOutputTypeSig.Meta))

		if expressionOperatorOutputArg.Type != types.UNDEFINED {
			return expressionOperatorOutputTypeSig
		}
	} else {
		return expressionOperatorOutputTypeSig
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0548.
	expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])

	return expressionInputTypeSig
}

// ShortDeclarationAssignment handles short assignments for ":=" operators.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	pkg - the package the expression belongs.
//	toExprs - Contains the output cx arg to be added to the expression.
//	fromExprs - Contains the output cx arg to be added to the expression.
func shortDeclarationAssignment(prgrm *ast.CXProgram, pkg *ast.CXPackage, toExprs []ast.CXExpression, fromExprs []ast.CXExpression, outputIndex int) []ast.CXExpression {
	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	lastFromExpressionIdx := len(fromExprs) - 1
	fromExpressionIdx := fromExprs[lastFromExpressionIdx].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0549.
	fromExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[fromExpressionIdx].Operator)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0550.
	expr := ast.MakeAtomicOperatorExpression(prgrm, nil)
	expressionIdx := expr.Index
	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)
	// nil for an atomic output.
	var sym *ast.CXArgument

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0552.
	toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])

	fn, err := prgrm.GetCurrentFunction()
	if err != nil {
		panic(err)
	}

	// Add name in fn.LocalVariables since its a new local declaration
	err = fn.AddLocalVariableName(toExpressionOutputTypeSig.Name)
	if err != nil {
		panic(err)
	}

	var typeSigIdx ast.CXTypeSignatureIndex
	if fromExpressionOperator == nil {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0553.
		fromExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)[0])
		var fromExpressionOutputArg *ast.CXArgument
		if fromExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0555.
			fromExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionOutputTypeSig.Meta))

			// then it's a literal
			_, sym = ast.MakeArgument(prgrm, toExpressionOutputTypeSig.Name, CurrentFile, LineNo)
			sym.SetType(fromExpressionOutputArg.Type)
			if len(fromExpressionOutputArg.Fields) > 0 {
				// x := f().y, which ProcessShortDeclaration types from the field.
				sym.Type, sym.Size = types.IDENTIFIER, types.IDENTIFIER.Size()
			} else if fromExpressionOutputArg.Type == types.STRUCT && fromExpressionOutputArg.StructType != nil {
				// A struct literal value.
				shortDeclarationStructTypeSet(sym, types.STRUCT, fromExpressionOutputArg.StructType)
				sym.Size = fromExpressionOutputArg.Size
				if fromExpressionOutputArg.PassBy == constants.PASSBY_REFERENCE {
					// &T{...}
					shortDeclarationPointerSet(sym)
				}
			}
			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
		} else if t, ok := shortDeclarationPointerType(prgrm, fromExpressionOutputTypeSig); ok && fromExpressionOutputTypeSig.PassBy == constants.PASSBY_REFERENCE {
			// x := &p of a pointer p is a pointer to a pointer.
			_, sym = ast.MakeArgument(prgrm, toExpressionOutputTypeSig.Name, CurrentFile, LineNo)
			sym.Type, sym.PointerTargetType, sym.Size, sym.StructType = t.typ, t.pointerTargetType, t.size, t.structType
			sym.DeclarationSpecifiers, sym.Lengths = t.specs, t.lengths
			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true
			typeSigIdx = ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(fromExpressionOutputTypeSig)
			newTypeSig.Name = toExpressionOutputTypeSig.Name
			typeSigIdx = newTypeSig.Index
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(fromExpressionOutputTypeSig)
			newTypeSig.Name = toExpressionOutputTypeSig.Name
			typeSigIdx = newTypeSig.Index
		} else {
			panic("type is not known")
		}

	} else {
		outTypeSig := getOutputTypeAt(prgrm, &fromExprs[lastFromExpressionIdx], outputIndex)
		if fromExprs[lastFromExpressionIdx].IsArrayLiteral() {
			// The value of an array or slice literal has its type.
			outTypeSig = prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)[0])
		}

		if outTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0564.
			outTypeArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outTypeSig.Meta))
			outType := outTypeArg.Type
			outTypeIsSlice := outTypeArg.IsSlice()

			_, sym = ast.MakeArgument(prgrm, toExpressionOutputTypeSig.Name, CurrentFile, LineNo)
			sym.SetType(outType)
			if outType == types.UNDEFINED && shortDeclarationOperatorChained(prgrm, fromExprs) {
				// x := a + b + c, which ProcessShortDeclaration types from the
				// operand a + b once that has its type.
				sym.Type, sym.Size = types.IDENTIFIER, types.IDENTIFIER.Size()
			}

			if fromExprs[lastFromExpressionIdx].IsArrayLiteral() {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0566.
				fromExpressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetInputs(prgrm)[0])
				var fromExpressionInputArg *ast.CXArgument
				if fromExpressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0568.
					fromExpressionInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionInputTypeSig.Meta))
				} else {
					panic("type is not cx argument deprecate\n\n")
				}

				fromCXAtomicOpInputs := fromExpressionInputArg
				sym.Size = fromCXAtomicOpInputs.Size
				sym.Lengths = fromCXAtomicOpInputs.Lengths
			}
			resultTyped := shortDeclarationResultTyped(fromExprs[lastFromExpressionIdx], fromExpressionOperator, outTypeArg) || fromExprs[lastFromExpressionIdx].IsArrayLiteral() && literalTypeCompound(outTypeArg)
			if resultTyped {
				// A declared result, as *str, []*T or [N][]T, gives its whole type.
				argumentTypeCopy(sym, outTypeArg)
			} else if outTypeIsSlice && len(outTypeArg.Lengths) > 1 {
				// A nested slice literal, [][]i32{{1}}, keeps every dimension.
				sym.Lengths = append([]types.Pointer(nil), outTypeArg.Lengths...)
				sym.DeclarationSpecifiers = append([]int(nil), outTypeArg.DeclarationSpecifiers...)
			} else if outTypeIsSlice {
				sym.Lengths = append([]types.Pointer{0}, sym.Lengths...)
				sym.DeclarationSpecifiers = append(sym.DeclarationSpecifiers, constants.DECL_SLICE)
			} else if ast.IsTypeArrayAtomic(outTypeArg) || ast.IsTypePointerArrayAtomic(outTypeArg) {
				sym.Lengths = outTypeArg.Lengths
				sym.DeclarationSpecifiers = append([]int(nil), outTypeArg.DeclarationSpecifiers...)
				sym.PointerTargetType = outTypeArg.PointerTargetType
			} else if outTypeArg.StructType != nil && len(outTypeArg.Lengths) > 0 {
				// An array of structs.
				sym.Lengths = outTypeArg.Lengths
				sym.DeclarationSpecifiers = append([]int(nil), outTypeArg.DeclarationSpecifiers...)
			}
			if !resultTyped && outType == types.STRUCT && outTypeArg.StructType != nil {
				sym.StructType = outTypeArg.StructType
				sym.Size = outTypeArg.StructType.GetStructSize(prgrm)
			}

			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true
			sym.Offset = outTypeArg.Offset

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
		} else if outTypeSig.Type == ast.TYPE_ATOMIC || outTypeSig.Type == ast.TYPE_ARRAY_ATOMIC || outTypeSig.Type == ast.TYPE_SLICE_ATOMIC || outTypeSig.Type == ast.TYPE_STRUCT || outTypeSig.Type == ast.TYPE_POINTER_STRUCT || outTypeSig.Type == ast.TYPE_POINTER_ARRAY_ATOMIC || outTypeSig.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(outTypeSig)
			newTypeSig.Name = toExpressionOutputTypeSig.Name
			typeSigIdx = newTypeSig.Index
		} else if outTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// TODO: recheck if this should be pointer
			// or the atomic type its pointing to.
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(outTypeSig)
			newTypeSig.Name = toExpressionOutputTypeSig.Name
			typeSigIdx = newTypeSig.Index
		} else {
			panic("type is not known")
		}

	}

	// The declaration is at the name, where its errors point (#486), not at
	// the value or a result it copied its type from.
	if file, line, column := ast.CXArgumentDebugPosition(prgrm, toExpressionOutputTypeSig.ArgDetails); line > 0 {
		detailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		details := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(detailsGid)
		details.FileName, details.FileLine, details.FileColumn = file, line, column
		prgrm.GetCXTypeSignatureFromArray(typeSigIdx).ArgDetails = detailsGid
	}

	prgrm.CXAtomicOps[expressionIdx].AddOutput(prgrm, typeSigIdx)

	// The type an atomic output gives its receivers is zero.
	var symType, symPointerTargetType types.Code
	var symSize types.Pointer
	if sym != nil {
		symType = sym.Type
		symPointerTargetType = sym.PointerTargetType
		symSize = sym.Size
		prgrm.ReleaseCXArgUnlessNamed(sym, typeSigIdx)
	}

	for _, toExpr := range toExprs {
		if toExpr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0576.
		toExprAtomicOp, err := prgrm.GetCXAtomicOp(toExpr.Index)
		if err != nil {
			panic(err)
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0577.
		toExprAtomicOpOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExprAtomicOp.GetOutputs(prgrm)[0])
		var toExprAtomicOpOutputIdx ast.CXArgumentIndex
		if toExprAtomicOpOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			toExprAtomicOpOutputIdx = ast.CXArgumentIndex(toExprAtomicOpOutputTypeSig.Meta)
		} else {
			panic("type is not type cx argument deprecate\n\n")
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1641.
		prgrm.GetCXArg(ast.CXArgumentIndex(toExprAtomicOpOutputIdx)).PreviouslyDeclared = true

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1642.
		prgrm.GetCXArg(ast.CXArgumentIndex(toExprAtomicOpOutputIdx)).Type = symType
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1643.
		prgrm.GetCXArg(ast.CXArgumentIndex(toExprAtomicOpOutputIdx)).PointerTargetType = symPointerTargetType
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1644.
		prgrm.GetCXArg(ast.CXArgumentIndex(toExprAtomicOpOutputIdx)).Size = symSize
	}

	// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
	// Now a slice of objects, site S0578.
	return append([]ast.CXExpression{*expr}, toExprs...)
}

func processAssignment(prgrm *ast.CXProgram, toExprs []ast.CXExpression, fromExprs []ast.CXExpression, toExpression *ast.CXAtomicOperator) []ast.CXExpression {
	lastFromExpressionIdx := len(fromExprs) - 1
	fromExpressionIdx := fromExprs[lastFromExpressionIdx].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0579.
	fromExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[fromExpressionIdx].Operator)

	toLastExpr, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, len(toExprs)-1)
	if err != nil {
		panic(err)
	}

	if fromExpressionOperator == nil {
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now an arena insert, site S0580.
		opIdx := prgrm.AddNativeFunctionInArray(ast.NativeFunctionOf(prgrm, constants.OP_IDENTITY))
		prgrm.CXAtomicOps[fromExpressionIdx].Operator = opIdx

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0581.
		toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])
		var toExpressionOutputIdx ast.CXArgumentIndex = -1
		if toExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			toExpressionOutputIdx = ast.CXArgumentIndex(toExpressionOutputTypeSig.Meta)
		} else if toExpressionOutputTypeSig.Type != ast.TYPE_ATOMIC && toExpressionOutputTypeSig.Type != ast.TYPE_POINTER_ATOMIC {
			panic("type is not type cx argument deprecate\n\n")
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0582.
		fromExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)[0])
		if toExpressionOutputIdx == -1 {
			// A typed target, such as a package global, keeps its type.
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0583.
			fromExpressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionOutputTypeSig.Meta))

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1645.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = fromExpressionOutputArg.Size
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1646.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = fromExpressionOutputArg.Type
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1647.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = fromExpressionOutputArg.PointerTargetType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1648.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = fromExpressionOutputArg.Lengths
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1649.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PassBy = fromExpressionOutputArg.PassBy
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1650.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = types.Code(fromExpressionOutputTypeSig.Meta).Size()
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1651.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(fromExpressionOutputTypeSig.Meta)
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1652.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = types.Code(fromExpressionOutputTypeSig.Meta).Size()
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1653.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.POINTER
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1654.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = types.Code(fromExpressionOutputTypeSig.Meta)
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
			// f()[i] on a slice result reads an element.
			eltType := types.Code(prgrm.GetCXTypeSignatureArrayFromArray(fromExpressionOutputTypeSig.Meta).Meta)
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = eltType.Size()
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = eltType
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[fromExpressionIdx].Inputs = prgrm.CXAtomicOps[fromExpressionIdx].Outputs

		prgrm.CXAtomicOps[fromExpressionIdx].Outputs = toLastExpr.Outputs

		return append(toExprs[:len(toExprs)-1], fromExprs...)
	} else {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0584.
		toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])
		var toExpressionOutputIdx ast.CXArgumentIndex
		if assignmentTargetAccess(prgrm, toExpressionOutputTypeSig) {
			// An element or a field, as in a[i] = f(), keeps its type.
		} else if toExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			toExpressionOutputIdx = ast.CXArgumentIndex(toExpressionOutputTypeSig.Meta)

			fromExpressionOperatorOutputs := fromExpressionOperator.GetOutputs(prgrm)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0585.
			fromExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(fromExpressionOperatorOutputs[0])
			if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0586.
				fromCXAtomicOpOperatorOutput := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionOperatorOutputTypeSig.Meta))

				if fromExpressionOperator.IsBuiltIn() {
					// only assigning as if the operator had only one output defined
					if fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
						// it's a short variable declaration
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1655.
						prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = fromCXAtomicOpOperatorOutput.Size
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1656.
						prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = fromCXAtomicOpOperatorOutput.Type
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1657.
						prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = fromCXAtomicOpOperatorOutput.PointerTargetType
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1658.
						prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = fromCXAtomicOpOperatorOutput.Lengths
					}

					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1659.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PassBy = fromCXAtomicOpOperatorOutput.PassBy
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1660.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = fromCXAtomicOpOperatorOutput.Size
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1661.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = fromCXAtomicOpOperatorOutput.Type
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1662.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = fromCXAtomicOpOperatorOutput.PointerTargetType
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1663.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = fromCXAtomicOpOperatorOutput.Lengths
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1664.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PassBy = fromCXAtomicOpOperatorOutput.PassBy
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1665.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(fromExpressionOperatorOutputTypeSig.Meta)
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1666.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(fromExpressionOperatorOutputTypeSig.Meta)
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1667.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(fromExpressionOperatorOutputTypeSig.Meta)
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1668.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.POINTER
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1669.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = types.Code(fromExpressionOperatorOutputTypeSig.Meta)
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S0587.
				sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(fromExpressionOperatorOutputTypeSig.Meta)

				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1670.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(sliceDetails.Meta)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1671.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = sliceDetails.Lengths
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1672.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(sliceDetails.Meta)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1673.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = sliceDetails.Lengths
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_STRUCT {
				// it's a short variable declaration
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1674.
				prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.STRUCT
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_STRUCT {
				assignmentPointerStructTarget(prgrm, prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)), fromExpressionOperatorOutputTypeSig)
			} else if !typeSignatureCollection(fromExpressionOperatorOutputTypeSig) {
				panic("type is not known")
			}
		} else if toExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || toExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			fromExpressionOperatorOutputs := fromExpressionOperator.GetOutputs(prgrm)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0588.
			fromExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(fromExpressionOperatorOutputs[0])
			if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0589.
				fromCXAtomicOpOperatorOutput := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionOperatorOutputTypeSig.Meta))
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					toExpressionOutputTypeSig.Meta = int(fromCXAtomicOpOperatorOutput.Type)
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					toExpressionOutputTypeSig.Meta = int(fromCXAtomicOpOperatorOutput.Type)
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					toExpressionOutputTypeSig.Meta = fromExpressionOperatorOutputTypeSig.Meta
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					toExpressionOutputTypeSig.Meta = fromExpressionOperatorOutputTypeSig.Meta
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					toExpressionOutputTypeSig.Meta = fromExpressionOperatorOutputTypeSig.Meta
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					toExpressionOutputTypeSig.Meta = fromExpressionOperatorOutputTypeSig.Meta
				}
			} else {
				panic("type is not known")
			}
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[fromExpressionIdx].Outputs = toLastExpr.Outputs

		return append(toExprs[:len(toExprs)-1], fromExprs...)
	}
}

// checkIfFunctionCallNeedsAnOutput records an error and returns true when the
// value assigned is a call of a function without outputs.
func checkIfFunctionCallNeedsAnOutput(prgrm *ast.CXProgram, fromExpressionOperator *ast.CXFunction, toExpression *ast.CXAtomicOperator) bool {
	fromExpressionOperatorOutputs := fromExpressionOperator.GetOutputs(prgrm)

	// Checking if we're trying to assign stuff from a function call
	// And if that function call actually returns something. If not, throw an error.
	if fromExpressionOperator != nil && len(fromExpressionOperatorOutputs) == 0 {
		toExpressionOutputs := toExpression.GetOutputs(prgrm)
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0590.
		toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpressionOutputs[0])

		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeAssignOutputlessOperator, toExpressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{})
		return true
	}
	return false
}

// Assignment handles assignment statements with different operators,
// like =, :=, +=, *=.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	toExprs, fromExprs - array of expressions where the assingment
//			  		  	 expression will be added.
//	assignOp - the assignment operator, "=", ":=", ">>=","<<=",
//			   "+=","-=","*=","/=","%=","&=","^=", and "|=".
func Assignment(prgrm *ast.CXProgram, toExprs []ast.CXExpression, assignOp string, fromExprs []ast.CXExpression) []ast.CXExpression {
	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	lastFromExpressionIdx := len(fromExprs) - 1
	fromExpressionIdx := fromExprs[lastFromExpressionIdx].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0591.
	fromExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[fromExpressionIdx].Operator)

	// A target that does not exist, such as pkg.x of a package whose import
	// failed, has its error already, and only a computed value is kept.
	if target, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, len(toExprs)-1); err == nil && len(target.GetOutputs(prgrm)) == 0 && functionInvalid(prgrm, prgrm.GetFunctionFromArray(target.Operator)) {
		if fromExpressionOperator == nil {
			return nil
		}
		return fromExprs
	}

	if checkIfFunctionCallNeedsAnOutput(prgrm, fromExpressionOperator, toExpression) {
		// The call stays as a statement, so the errors after it are found too.
		if assignOp != ":=" {
			return fromExprs
		}
		// y := noout() still declares y, as a placeholder, so its uses are no errors.
		target := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])
		file, line := ast.CXArgumentDebugFileGidLine(prgrm, target.ArgDetails)
		_, declarator := ast.MakeArgument(prgrm, target.Name, file, line)
		placeholder := declarationSpecifiersPlaceholder(prgrm, file, line)
		placeholder.Type = types.INVALID
		return append(DeclareLocal(prgrm, declarator, placeholder, nil, false), fromExprs...)
	}

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0592.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	switch assignOp {
	case ":=":
		shortDeclExprs := shortDeclarationAssignment(prgrm, pkg, toExprs, fromExprs, 0)
		// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
		// Now a slice of objects, site S0593.
		toExprs = append([]ast.CXExpression{*exprCXLine}, shortDeclExprs...)
	case ">>=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0594.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_BITSHR))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "<<=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0595.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_BITSHL))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "+=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0596.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_ADD))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "-=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0597.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_SUB))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "*=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0598.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_MUL))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "/=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0599.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_DIV))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "%=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0600.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_MOD))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "&=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0601.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_BITAND))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "^=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0602.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_BITXOR))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "|=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0603.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.NativeFunctionOf(prgrm, constants.OP_BITOR))
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	}

	declarationCount := len(toExprs) - 1
	// x := f().y reads the result of f, so the declaration moves after the call.
	valueRead := fromExpressionOperator == nil && expressionsCount(fromExprs) > 1
	exprs := processAssignment(prgrm, toExprs, fromExprs, toExpression)
	if assignOp == ":=" && (valueRead || methodCallFound(fromExprs) || shortDeclarationOperatorChained(prgrm, fromExprs) && shortDeclarationUntyped(prgrm, toExprs)) {
		exprs = shortDeclarationAfterMethodCalls(exprs, declarationCount)
	}
	return exprs
}

// ShortDeclarationShare marks the declarations exprs make for the names
// targets of a := with more than one name, so each assigns a variable the
// same scope already has, and only a := with no new name is an error.
func ShortDeclarationShare(prgrm *ast.CXProgram, targets [][]ast.CXExpression, exprs []ast.CXExpression) {
	var names []stringpool.StringGid
	for _, target := range targets {
		op, err := prgrm.GetCXAtomicOpFromExpressions(target, len(target)-1)
		if err != nil || prgrm.GetFunctionFromArray(op.Operator) != nil || len(op.GetOutputs(prgrm)) != 1 {
			continue
		}
		names = append(names, prgrm.GetCXTypeSignatureFromArray(op.GetOutputs(prgrm)[0]).Name)
	}
	last := -1
	for i := range exprs {
		if len(names) == 0 {
			break
		}
		if exprs[i].Type == ast.CX_LINE || exprs[i].ExpressionType != ast.CXEXPR_UNUSED {
			continue
		}
		op := &prgrm.CXAtomicOps[exprs[i].Index]
		if prgrm.GetFunctionFromArray(op.Operator) != nil || len(op.GetInputs(prgrm)) != 0 || len(op.GetOutputs(prgrm)) != 1 {
			continue
		}
		if prgrm.GetCXTypeSignatureFromArray(op.GetOutputs(prgrm)[0]).Name != names[0] {
			continue
		}
		exprs[i].ExpressionType = ast.CXEXPR_SHORT_DECLARATION_SHARED
		last = i
		names = names[1:]
	}
	if last >= 0 {
		exprs[last].ExpressionType = ast.CXEXPR_SHORT_DECLARATION_SHARED_LAST
	}
}

// methodCallFound reports whether exprs hold a method call.
func methodCallFound(exprs []ast.CXExpression) bool {
	for _, expr := range exprs {
		if expr.IsMethodCall() {
			return true
		}
	}
	return false
}

// shortDeclarationAfterMethodCalls moves the declaration of a := to just
// before its assignment, which ProcessShortDeclaration types from it.
func shortDeclarationAfterMethodCalls(exprs []ast.CXExpression, declarationCount int) []ast.CXExpression {
	last := len(exprs) - 1
	if declarationCount <= 0 || declarationCount > last {
		return exprs
	}
	reordered := make([]ast.CXExpression, 0, len(exprs))
	reordered = append(reordered, exprs[declarationCount:last]...)
	reordered = append(reordered, exprs[:declarationCount]...)
	return append(reordered, exprs[last])
}

// AssignmentStatement is Assignment for an assignment statement. A method
// call assigned with = writes its result to the target, with no copy.
func AssignmentStatement(prgrm *ast.CXProgram, toExprs []ast.CXExpression, assignOp string, fromExprs []ast.CXExpression) []ast.CXExpression {
	if assignOp == "=" {
		if callExprs := MethodCallStatement(prgrm, fromExprs); len(callExprs) < len(fromExprs) {
			toLastExpr, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, len(toExprs)-1)
			if err != nil {
				panic(err)
			}
			prgrm.CXAtomicOps[callExprs[len(callExprs)-1].Index].Outputs = toLastExpr.Outputs
			return append(toExprs[:len(toExprs)-1], callExprs...)
		}
	}
	return Assignment(prgrm, toExprs, assignOp, fromExprs)
}

// MultipleValueName returns a new name for the variable that holds one value
// of a multiple assignment.
func MultipleValueName(prgrm *ast.CXProgram) stringpool.StringGid {
	return generateTempVarName(prgrm, constants.MULTIPLE_VALUE_PREFIX)
}

// MultipleValueRead returns the expressions that read the variable name.
func MultipleValueRead(prgrm *ast.CXProgram, name stringpool.StringGid) []ast.CXExpression {
	return primaryIdentifierExpressions(prgrm, name)
}

// MultipleAssignmentCall assigns the outputs of the call that ends value to
// targets, as in a, b = f() and a, b := f().
func MultipleAssignmentCall(prgrm *ast.CXProgram, targets [][]ast.CXExpression, assignOp string, value []ast.CXExpression) []ast.CXExpression {
	if value[len(value)-1].Type != ast.CX_LINE && len(value) >= 2 && value[len(value)-2].IsMethodCall() {
		return multipleAssignmentMethodCall(prgrm, targets, assignOp, value)
	}
	callIdx := value[len(value)-1].Index
	operator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[callIdx].Operator)
	valueCount := 1
	if operator != nil && len(prgrm.CXAtomicOps[callIdx].GetOutputs(prgrm)) == 0 {
		valueCount = len(operator.GetOutputs(prgrm))
	}
	if valueCount != len(targets) {
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeAssignmentCount, CurrentFile, LineNo, ast.CompileErrorOperands{int64(len(targets)), int64(valueCount)})
		if assignOp == ":=" {
			return ShortDeclarationTargetsInvalid(prgrm, targets)
		}
		return nil
	}

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	var result []ast.CXExpression
	if assignOp == ":=" {
		result = append(result, *MakeCXLineExpressionCurrent(prgrm))
	}
	for i, target := range targets {
		if assignOp == ":=" {
			declExprs := shortDeclarationAssignment(prgrm, pkg, target, value, i)
			result = append(result, declExprs[0])
		} else {
			result = append(result, target[:len(target)-1]...)
		}
		targetOp, err := prgrm.GetCXAtomicOpFromExpressions(target, len(target)-1)
		if err != nil {
			panic(err)
		}
		prgrm.CXAtomicOps[callIdx].AddOutput(prgrm, targetOp.GetOutputs(prgrm)[0])
	}
	return append(result, value...)
}

// multipleAssignmentMethodCall gives a method call one result variable per
// target, which ProcessMethodCall types, and assigns each to its target.
func multipleAssignmentMethodCall(prgrm *ast.CXProgram, targets [][]ast.CXExpression, assignOp string, value []ast.CXExpression) []ast.CXExpression {
	callOp := &prgrm.CXAtomicOps[value[len(value)-2].Index]
	readOp := &prgrm.CXAtomicOps[value[len(value)-1].Index]
	names := []stringpool.StringGid{prgrm.GetCXTypeSignatureFromArray(readOp.GetOutputs(prgrm)[0]).Name}
	for len(names) < len(targets) {
		names = append(names, methodCallResultAdd(prgrm, callOp))
	}
	result := append([]ast.CXExpression(nil), value[:len(value)-1]...)
	for i, target := range targets {
		result = append(result, AssignmentStatement(prgrm, target, assignOp, primaryIdentifierExpressions(prgrm, names[i]))...)
	}
	return result
}

// assignmentPointerStructTarget types the target of x = f() where f returns
// the *T of sig.
func assignmentPointerStructTarget(prgrm *ast.CXProgram, target *ast.CXArgument, sig *ast.CXTypeSignature) {
	target.Type = types.POINTER
	target.PointerTargetType = types.STRUCT
	target.StructType = prgrm.GetCXTypeSignatureStructFromArray(sig.Meta).StructType
	target.Size = types.POINTER_SIZE
}

// expressionsCount returns the number of expressions in exprs that are not
// lines.
func expressionsCount(exprs []ast.CXExpression) int {
	count := 0
	for _, expr := range exprs {
		if expr.Type != ast.CX_LINE {
			count++
		}
	}
	return count
}

// shortDeclarationOperatorChained reports whether the value exprs of a := is
// an operator of the value of another expression, as a + b + c is, whose type
// is not known until that expression has its type.
func shortDeclarationOperatorChained(prgrm *ast.CXProgram, exprs []ast.CXExpression) bool {
	if expressionsCount(exprs) < 2 || exprs[len(exprs)-1].Type == ast.CX_LINE {
		return false
	}
	operator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[exprs[len(exprs)-1].Index].Operator)
	return operator != nil && ast.IsOperator(operator.AtomicOPCode)
}

// shortDeclarationUntyped reports whether the declaration of a :=, the first
// expression of toExprs after its line, waits for ProcessShortDeclaration to
// give it a type.
func shortDeclarationUntyped(prgrm *ast.CXProgram, toExprs []ast.CXExpression) bool {
	for _, expr := range toExprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		outputs := prgrm.CXAtomicOps[expr.Index].GetOutputs(prgrm)
		if len(outputs) == 0 {
			return false
		}
		sig := prgrm.GetCXTypeSignatureFromArray(outputs[0])
		return sig.Type == ast.TYPE_CXARGUMENT_DEPRECATE && prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta)).Type == types.IDENTIFIER
	}
	return false
}

// shortDeclarationResultTyped reports whether outArg, the result x := f()
// declares x from, is a declared result of a function, with no indexes or
// fields, whose whole type x takes.
func shortDeclarationResultTyped(from ast.CXExpression, operator *ast.CXFunction, outArg *ast.CXArgument) bool {
	if operator == nil || operator.IsBuiltIn() || from.IsArrayLiteral() || outArg.Type == types.UNDEFINED {
		return false
	}
	return len(outArg.Indexes) == 0 && len(outArg.Fields) == 0 && len(outArg.DereferenceOperations) == 0
}

// argumentTypeCopy gives dst the type of src, with its own slices.
func argumentTypeCopy(dst, src *ast.CXArgument) {
	dst.Type, dst.PointerTargetType, dst.Size = src.Type, src.PointerTargetType, src.Size
	dst.Lengths = append([]types.Pointer(nil), src.Lengths...)
	dst.DeclarationSpecifiers = append([]int(nil), src.DeclarationSpecifiers...)
	dst.StructType, dst.PassBy = src.StructType, src.PassBy
}

// typeSignatureCollection reports whether sig is an array or slice, or a
// pointer to one, whose target takes its type from its declaration.
func typeSignatureCollection(sig *ast.CXTypeSignature) bool {
	switch sig.Type {
	case ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		return true
	}
	return false
}

// assignmentTargetAccess reports whether sig, the target of an assignment,
// is an index or a field access, as a[i] and s.f are.
func assignmentTargetAccess(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) bool {
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return false
	}
	arg := prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta))
	return len(arg.Indexes) > 0 || len(arg.Fields) > 0
}

// ShortDeclarationTargetsInvalid declares each target of a := whose values
// do not match it as a types.INVALID local, so its uses report nothing more.
// A target that names a variable of the function already is left alone, as
// := assigns it in Go (https://go.dev/ref/spec#Short_variable_declarations).
func ShortDeclarationTargetsInvalid(prgrm *ast.CXProgram, targets [][]ast.CXExpression) []ast.CXExpression {
	fn, err := prgrm.GetCurrentFunction()
	if err != nil {
		return nil
	}
	var result []ast.CXExpression
	for _, target := range targets {
		last := len(target) - 1
		if last < 0 || target[last].Type == ast.CX_LINE {
			continue
		}
		expression := &prgrm.CXAtomicOps[target[last].Index]
		outputs := expression.GetOutputs(prgrm)
		if prgrm.GetFunctionFromArray(expression.Operator) != nil || len(outputs) != 1 || len(expression.GetInputs(prgrm)) > 0 {
			continue
		}
		sig := prgrm.GetCXTypeSignatureFromArray(outputs[0])
		if fn.IsLocalVariable(sig.Name) || functionParameterNamed(prgrm, fn, sig.Name) {
			continue
		}
		file, line := ast.CXArgumentDebugFileGidLine(prgrm, sig.ArgDetails)
		_, declarator := ast.MakeArgument(prgrm, sig.Name, file, line)
		result = append(result, DeclareLocal(prgrm, declarator, declarationSpecifiersMissing(prgrm, file, line), nil, false)...)
	}
	return result
}

// functionParameterNamed reports whether fn has a parameter or result named
// name.
func functionParameterNamed(prgrm *ast.CXProgram, fn *ast.CXFunction, name stringpool.StringGid) bool {
	for _, sigIdx := range append(append([]ast.CXTypeSignatureIndex(nil), fn.GetInputs(prgrm)...), fn.GetOutputs(prgrm)...) {
		if prgrm.GetCXTypeSignatureFromArray(sigIdx).Name == name {
			return true
		}
	}
	return false
}

// ExpressionsDeclareInvalid reports whether exprs end in the declaration of
// a types.INVALID placeholder, as ShortDeclarationTargetsInvalid makes.
func ExpressionsDeclareInvalid(prgrm *ast.CXProgram, exprs []ast.CXExpression) bool {
	last := len(exprs) - 1
	if last < 0 || exprs[last].Type == ast.CX_LINE {
		return false
	}
	expression := &prgrm.CXAtomicOps[exprs[last].Index]
	outputs := expression.GetOutputs(prgrm)
	return prgrm.GetFunctionFromArray(expression.Operator) == nil && len(expression.GetInputs(prgrm)) == 0 && len(outputs) == 1 && typeSignatureInvalid(prgrm, prgrm.GetCXTypeSignatureFromArray(outputs[0]))
}
