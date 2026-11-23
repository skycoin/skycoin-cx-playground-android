package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin/src/cipher/encoder"
)

// SliceLiteralExpression handles literal expressions by converting it to a series of `append` expressions.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 typeCode - type code of the slice.
//	 exprs - array of expressions that make up the slice literal expression.
func SliceLiteralExpression(prgrm *ast.CXProgram, typeCode types.Code, exprs []ast.CXExpression) []ast.CXExpression {
	return sliceLiteralExpressionOf(prgrm, typeCode, nil, exprs)
}

// sliceLiteralSetType gives arg the element type of a slice literal, the
// struct strct when it is set. Only a declaration names the struct type.
func sliceLiteralSetType(prgrm *ast.CXProgram, arg *ast.CXArgument, typeCode types.Code, strct *ast.CXStruct, declaration bool) {
	if strct == nil {
		arg.SetType(typeCode)
		return
	}
	arg.Type = types.STRUCT
	arg.StructType = strct
	arg.Size = strct.GetStructSize(prgrm)
	if declaration {
		arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_STRUCT)
	}
}

// sliceLiteralExpressionOf is SliceLiteralExpression for elements of type
// typeCode, or of the struct strct when it is set.
func sliceLiteralExpressionOf(prgrm *ast.CXProgram, typeCode types.Code, strct *ast.CXStruct, exprs []ast.CXExpression) []ast.CXExpression {
	var result []ast.CXExpression

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	symName := generateTempVarName(prgrm, constants.SLICE_LITERAL_PREFIX)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0980.
	slcVarExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// adding the declaration
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0981.
	slcVarExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
	slcVarExprAtomicOpIdx := slcVarExpr.Index

	prgrm.CXAtomicOps[slcVarExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)
	_, slcVar := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
	sliceLiteralSetType(prgrm, slcVar, typeCode, strct, true)
	slcVar = DeclarationSpecifiers(slcVar, []types.Pointer{0}, constants.DECL_SLICE)
	slcVar.Package = ast.CXPackageIndex(pkg.Index)
	slcVar.PreviouslyDeclared = true

	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, slcVar)
	prgrm.CXAtomicOps[slcVarExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

	result = append(result, *slcVarExprCXLine, *slcVarExpr)

	var endPointsCounter int
	for i, expr := range exprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0984.
		exprAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0985.
		exprAtomicOpOperator := prgrm.GetFunctionFromArray(exprAtomicOp.Operator)

		exprCXLine, _ := prgrm.GetPreviousCXLine(exprs, i)

		if expr.IsArrayLiteral() {
			expr.ExpressionType = ast.CXEXPR_UNUSED
			if exprAtomicOpOperator == nil && len(exprAtomicOp.GetOutputs(prgrm)) == 0 {
				// An element with no value, such as T{} in a []i32.
				prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeInvalidExpression, CurrentFile, LineNo, ast.CompileErrorOperands{})
				continue
			}

			_, symInp := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
			sliceLiteralSetType(prgrm, symInp, typeCode, strct, false)
			symInp.Package = ast.CXPackageIndex(pkg.Index)
			// The slice the element is appended to is at the element, where the type
			// errors of the append print.
			if exprCXLine.LineNumber > 0 && symInp.ArgDetails != ast.CXArgumentDebugGidNil {
				details := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(symInp.ArgDetails)
				details.FileName, details.FileLine, details.FileColumn = exprCXLine.FileName, exprCXLine.LineNumber, exprCXLine.LineColumn
			}

			_, symOut := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
			sliceLiteralSetType(prgrm, symOut, typeCode, strct, false)
			symOut.Package = ast.CXPackageIndex(pkg.Index)

			endPointsCounter++

			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S0988.
			symExprExprCXLine := MakeCXLineExpressionCurrent(prgrm)
			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S0989.
			symExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
			symExprAtomicOpIdx := symExpr.Index

			prgrm.CXAtomicOps[symExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)
			typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symOut)
			prgrm.CXAtomicOps[symExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

			if exprAtomicOpOperator == nil {
				// then it's a literal
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now an arena insert, site S0991.
				opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_APPEND])
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Operator = opIdx

				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs = nil
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symInp)
				prgrm.CXAtomicOps[symExprAtomicOpIdx].AddInput(prgrm, typeSigIdx)
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs.Fields = append(prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs.Fields, exprAtomicOp.Outputs.Fields...)
			} else {
				// We need to create a temporary variable to hold the result of the
				// nested expressions. Then use that variable as part of the slice literal.

				outTypeSig := getOutputType(prgrm, &expr)
				if outputs := exprAtomicOp.GetOutputs(prgrm); len(outputs) > 0 {
					// A nested slice literal, whose output has its full type.
					outTypeSig = prgrm.GetCXTypeSignatureFromArray(outputs[0])
				}
				var outArg *ast.CXArgument
				var outTypeSigIdx ast.CXTypeSignatureIndex
				if outTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0993.
					outArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outTypeSig.Meta))

					_, out := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), exprCXLine.FileName, exprCXLine.LineNumber)
					out.SetType(outArg.Type)
					out.DeclarationSpecifiers = append(out.DeclarationSpecifiers[:0], outArg.DeclarationSpecifiers...)
					out.Lengths = outArg.Lengths
					out.PointerTargetType = outArg.PointerTargetType
					out.StructType = outArg.StructType
					out.Size = outArg.Size
					out.PreviouslyDeclared = true
					out.Package = ast.CXPackageIndex(pkg.Index)

					outTypeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
				} else if outTypeSig.Type == ast.TYPE_ATOMIC || outTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(outTypeSig)
					newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
					newTypeSig.Package = ast.CXPackageIndex(pkg.Index)
					outTypeSigIdx = newTypeSig.Index
				} else if outTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
					arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(outTypeSig.Meta)
					_, out := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), exprCXLine.FileName, exprCXLine.LineNumber)
					out.SetType(types.Code(arrDetails.Meta))
					out.DeclarationSpecifiers = declarationSpecifiersSlice(len(arrDetails.Lengths))
					out.Lengths = arrDetails.Lengths
					out.PreviouslyDeclared = true
					out.Package = ast.CXPackageIndex(pkg.Index)
					outTypeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
				} else {
					panic("type is not known")
				}

				// MakeAtomicOperatorExpression above may have grown CXAtomicOps, so refetch.
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0999.
				exprAtomicOp, err = prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}

				exprAtomicOp.Outputs = nil

				exprAtomicOp.AddOutput(prgrm, outTypeSigIdx)

				result = append(result, expr)
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now an arena insert, site S1000.
				opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_APPEND])
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Operator = opIdx

				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs = nil
				typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symInp)
				prgrm.CXAtomicOps[symExprAtomicOpIdx].AddInput(prgrm, typeSigIdx)
				prgrm.CXAtomicOps[symExprAtomicOpIdx].AddInput(prgrm, outTypeSigIdx)
			}
			result = append(result, *symExprExprCXLine, *symExpr)
		} else {
			result = append(result, expr)
		}
	}

	symNameOutput := generateTempVarName(prgrm, constants.LOCAL_PREFIX)

	_, symOutput := ast.MakeArgument(prgrm, symNameOutput, CurrentFile, LineNo)
	sliceLiteralSetType(prgrm, symOutput, typeCode, strct, true)
	symOutput = DeclarationSpecifiers(symOutput, []types.Pointer{0}, constants.DECL_SLICE)
	symOutput.Package = ast.CXPackageIndex(pkg.Index)
	symOutput.PreviouslyDeclared = true

	_, symInput := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
	sliceLiteralSetType(prgrm, symInput, typeCode, strct, false)
	symInput.Package = ast.CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1004.
	symExprExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1005.
	symExpr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1006.
	symExprAtomicOp, err := prgrm.GetCXAtomicOp(symExpr.Index)
	if err != nil {
		panic(err)
	}

	symExprAtomicOp.Package = ast.CXPackageIndex(pkg.Index)
	typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symOutput)
	symExprAtomicOp.AddOutput(prgrm, typeSigIdx)
	typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symInput)
	symExprAtomicOp.AddInput(prgrm, typeSigIdx)

	// marking the output so multidimensional arrays identify the expressions
	symExpr.ExpressionType = ast.CXEXPR_ARRAY_LITERAL
	result = append(result, *symExprExprCXLine, *symExpr)

	return result
}

// SliceLiteralElement marks the last expression of one slice literal element.
// An element with no expressions, T{}, gets an expression with no output. A
// column above 0 puts a line at line and column, where the element starts,
// before the last expression, and the append of the element takes it.
func SliceLiteralElement(prgrm *ast.CXProgram, element []ast.CXExpression, line int, column int) []ast.CXExpression {
	if len(element) == 0 {
		element = []ast.CXExpression{*ast.MakeAtomicOperatorExpression(prgrm, nil)}
	}
	last := len(element) - 1
	element[last].ExpressionType = ast.CXEXPR_ARRAY_LITERAL
	if column > 0 {
		cxLine := ast.MakeCXLineExpression(prgrm, CurrentFile, line, prgrm.CXProgramNameIntern(LineStr))
		if position, err := prgrm.GetCXLine(cxLine.Index); err == nil {
			position.LineColumn = column
		}
		element = append(element[:last:last], *cxLine, element[last])
	}
	return element
}

// SliceLiteralStructExpression handles a slice literal of the struct
// structName, `[]T{T{x: 1}, t}`, as SliceLiteralExpression does for []i32.
func SliceLiteralStructExpression(prgrm *ast.CXProgram, structName string, line int, column int, exprs []ast.CXExpression) []ast.CXExpression {
	if column == 0 {
		line = LineNo
	}
	elt := DeclarationSpecifiersStruct(prgrm, structName, "", false, CurrentFile, line, column)
	if elt.StructType == nil {
		return structLiteralInvalid(prgrm)
	}
	return sliceLiteralExpressionOf(prgrm, types.STRUCT, elt.StructType, sliceLiteralStructElements(prgrm, elt.StructType, exprs, constants.ELEMENT_PREFIX))
}

// sliceLiteralStructElements gives each element of a literal of the struct strct
// its own variable named with prefix, as sliceLiteralStructElement does.
func sliceLiteralStructElements(prgrm *ast.CXProgram, strct *ast.CXStruct, exprs []ast.CXExpression, prefix string) []ast.CXExpression {
	var elements []ast.CXExpression
	start := 0
	for i := range exprs {
		if exprs[i].Type == ast.CX_LINE || !exprs[i].IsArrayLiteral() {
			continue
		}
		elements = append(elements, sliceLiteralStructElement(prgrm, strct, exprs[start:i+1], prefix)...)
		start = i + 1
	}
	return elements
}

// sliceLiteralStructElement assigns one element of a []strct literal to a new
// variable and ends with that variable, so it is appended like a literal.
func sliceLiteralStructElement(prgrm *ast.CXProgram, strct *ast.CXStruct, element []ast.CXExpression, prefix string) []ast.CXExpression {
	if sliceLiteralStructInner(prgrm, element) {
		return element
	}
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	elementName := generateTempVarName(prgrm, prefix)
	elementArg := func(declaration bool) ast.CXTypeSignatureIndex {
		_, arg := ast.MakeArgument(prgrm, elementName, CurrentFile, LineNo)
		sliceLiteralSetType(prgrm, arg, types.STRUCT, strct, declaration)
		arg.Package = ast.CXPackageIndex(pkg.Index)
		arg.PreviouslyDeclared = true
		// A named signature lets the symbols table take the declaration as a temp.
		return ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, arg)
	}

	declCXLine := MakeCXLineExpressionCurrent(prgrm)
	decl := ast.MakeAtomicOperatorExpression(prgrm, nil)
	prgrm.CXAtomicOps[decl.Index].Package = ast.CXPackageIndex(pkg.Index)
	prgrm.CXAtomicOps[decl.Index].AddOutput(prgrm, elementArg(true))
	result := []ast.CXExpression{*declCXLine, *decl}

	last := len(element) - 1
	for last > 0 && element[last].Type == ast.CX_LINE {
		last--
	}
	element[last].ExpressionType = ast.CXEXPR_UNUSED
	// The element written is at the element start, for the type error.
	elementAt := func() ast.CXTypeSignatureIndex {
		output := elementArg(false)
		if elementCXLine, _ := prgrm.GetPreviousCXLine(element, last); elementCXLine != nil && elementCXLine.LineNumber > 0 {
			if details := prgrm.GetCXTypeSignatureFromArray(output).ArgDetails; details != ast.CXArgumentDebugGidNil {
				debug := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(details)
				debug.FileName, debug.FileLine, debug.FileColumn = elementCXLine.FileName, elementCXLine.LineNumber, elementCXLine.LineColumn
			}
		}
		return output
	}
	switch {
	case len(prgrm.CXAtomicOps[element[last].Index].GetOutputs(prgrm)) == 0 && prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[element[last].Index].Operator) == nil:
		// T{} leaves the declared variable zero.
	case prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[element[last].Index].Operator) == nil:
		// A value: elementName = value, which type checks it.
		value := prgrm.CXAtomicOps[element[last].Index].GetOutputs(prgrm)
		result = append(result, element[:last]...)
		assign := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
		prgrm.CXAtomicOps[assign.Index].Package = ast.CXPackageIndex(pkg.Index)
		prgrm.CXAtomicOps[assign.Index].AddOutput(prgrm, elementAt())
		prgrm.CXAtomicOps[assign.Index].AddInput(prgrm, value[0])
		result = append(result, *assign)
	default:
		// A call writes its result to elementName.
		prgrm.CXAtomicOps[element[last].Index].Outputs = nil
		prgrm.CXAtomicOps[element[last].Index].AddOutput(prgrm, elementAt())
		result = append(result, element...)
	}

	use := ast.MakeAtomicOperatorExpression(prgrm, nil)
	prgrm.CXAtomicOps[use.Index].Package = ast.CXPackageIndex(pkg.Index)
	prgrm.CXAtomicOps[use.Index].AddOutput(prgrm, elementArg(false))
	use.ExpressionType = ast.CXEXPR_ARRAY_LITERAL
	return append(result, *use)
}

// PrimaryStructLiteral creates a struct literal value. Its fields are written
// to a new zeroed variable, which the last expression gives as the value. A
// missing struct is an error at line and column, where its name is, or at the
// token the parser is at for a column of 0.
func PrimaryStructLiteral(prgrm *ast.CXProgram, structName string, line int, column int, structFields []ast.CXExpression) []ast.CXExpression {
	name := generateTempVarName(prgrm, constants.LOCAL_PREFIX)
	writes, strct := primaryStructLiteralWrites(prgrm, structName, line, column, name, structFields)
	if strct == nil {
		return structLiteralInvalid(prgrm)
	}
	return structLiteralValue(prgrm, strct, name, writes)
}

// PrimaryStructLiteralReference creates &T{...}, the address of the value
// PrimaryStructLiteral makes, which escape analysis may move to the heap.
func PrimaryStructLiteralReference(prgrm *ast.CXProgram, operator string, structName string, structFields []ast.CXExpression) []ast.CXExpression {
	return UnaryExpression(prgrm, operator, PrimaryStructLiteral(prgrm, structName, LineNo, 0, structFields))
}

// primaryStructLiteralWrites makes each field write of a struct literal of
// structName write outputName.field, and returns the struct.
func primaryStructLiteralWrites(prgrm *ast.CXProgram, structName string, line int, column int, outputName stringpool.StringGid, structFields []ast.CXExpression) ([]ast.CXExpression, *ast.CXStruct) {
	var result []ast.CXExpression
	var structResult *ast.CXStruct
	var fieldNames []stringpool.StringGid
	structNameGid := prgrm.CXProgramNameIntern(structName)

	if pkg, err := prgrm.GetCurrentPackage(); err == nil {
		if strct, err := prgrm.GetStruct(structNameGid, pkg.Name); err == nil {
			structResult = strct
			for _, expr := range structFields {
				if expr.Type == ast.CX_LINE {
					continue
				}
				// A method call or a nested value is not a field write.
				if expr.ExpressionType != ast.CXEXPR_STRUCT_LITERAL_FIELD {
					result = append(result, expr)
					continue
				}
				structLiteralFieldRepeatedCheck(prgrm, &fieldNames, expr)
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S1009.
				cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}

				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S1010.
				cxAtomicOpOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(cxAtomicOp.GetOutputs(prgrm)[0])
				if cxAtomicOpOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					cxAtomicOpOutputIdx := cxAtomicOpOutputTypeSig.Meta
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1741.
					name := prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Name
					structLiteralValueTypeSet(prgrm, strct, prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)), name)

					_, field := ast.MakeArgument(prgrm, name, CurrentFile, LineNo)
					argumentDebugPositionCopy(prgrm, field.ArgDetails, cxAtomicOpOutputTypeSig.ArgDetails)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1742.
					field.Type = prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Type
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1743.
					field.PointerTargetType = prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).PointerTargetType
					field.StructType = strct

					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1744.
					prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Package = ast.CXPackageIndex(pkg.Index)

					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1745.
					if prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).StructType == nil {
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1746.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).StructType = strct
					}

					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1747.
					prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Size = strct.GetStructSize(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1748.
					prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Name = outputName
					cxAtomicOpOutputTypeSig.Name = outputName

					fieldIdx := ast.CXArgumentIndex(field.Index)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1749.
					prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Fields = append(prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Fields, fieldIdx)
				} else if cxAtomicOpOutputTypeSig.Type == ast.TYPE_ATOMIC {
					// TODO: give proper change when we implement type_structs
					// Looks like we have to convert the arg to type cx arg deprecate again
					argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
					argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
					argDetails.FileName = CurrentFile
					argDetails.FileLine = LineNo
					argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
					_, newCXArg := prgrm.AllocCXArgument()
					newCXArg.ArgDetails = argDetailsGid
					newCXArg.Type = types.Code(cxAtomicOpOutputTypeSig.Meta)
					newCXArg.Package = ast.CXPackageIndex(pkg.Index)
					newCXArg.Offset = cxAtomicOpOutputTypeSig.Offset

					newCXArg.StructType = strct
					newCXArg.Size = strct.GetStructSize(prgrm)
					newCXArg.Name = outputName

					_, field := ast.MakeArgument(prgrm, cxAtomicOpOutputTypeSig.Name, CurrentFile, LineNo)
					argumentDebugPositionCopy(prgrm, field.ArgDetails, cxAtomicOpOutputTypeSig.ArgDetails)
					field.Type = types.Code(cxAtomicOpOutputTypeSig.Meta)
					field.StructType = strct
					fieldIdx := ast.CXArgumentIndex(field.Index)

					newCXArg.Fields = append(newCXArg.Fields, fieldIdx)
					newCXArgIdx := ast.CXArgumentIndex(newCXArg.Index)

					cxAtomicOpOutputTypeSig.Name = outputName
					cxAtomicOpOutputTypeSig.Type = ast.TYPE_CXARGUMENT_DEPRECATE
					cxAtomicOpOutputTypeSig.Meta = int(newCXArgIdx)
				} else if cxAtomicOpOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					// TODO: give proper change when we implement type_structs
					// Looks like we have to convert the arg to type cx arg deprecate again
					argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
					argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
					argDetails.FileName = CurrentFile
					argDetails.FileLine = LineNo
					argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
					_, newCXArg := prgrm.AllocCXArgument()
					newCXArg.ArgDetails = argDetailsGid
					newCXArg.Type = types.Code(cxAtomicOpOutputTypeSig.Meta)
					newCXArg.Package = ast.CXPackageIndex(pkg.Index)
					newCXArg.Offset = cxAtomicOpOutputTypeSig.Offset
					newCXArg.DeclarationSpecifiers = []int{constants.DECL_BASIC, constants.DEREF_POINTER}

					newCXArg.StructType = strct
					newCXArg.Size = strct.GetStructSize(prgrm)
					newCXArg.Name = outputName

					_, field := ast.MakeArgument(prgrm, cxAtomicOpOutputTypeSig.Name, CurrentFile, LineNo)
					argumentDebugPositionCopy(prgrm, field.ArgDetails, cxAtomicOpOutputTypeSig.ArgDetails)
					field.Type = types.Code(cxAtomicOpOutputTypeSig.Meta)
					field.StructType = strct
					fieldIdx := ast.CXArgumentIndex(field.Index)

					newCXArg.Fields = append(newCXArg.Fields, fieldIdx)
					newCXArgIdx := ast.CXArgumentIndex(newCXArg.Index)

					cxAtomicOpOutputTypeSig.Name = outputName
					cxAtomicOpOutputTypeSig.Type = ast.TYPE_CXARGUMENT_DEPRECATE
					cxAtomicOpOutputTypeSig.Meta = int(newCXArgIdx)
				} else {
					panic("type is not known")
				}

				expr.ExpressionType = ast.CXEXPR_STRUCT_LITERAL
				result = append(result, expr)
			}
		} else {
			structLiteralMissing(prgrm, line, column, err)
		}
	} else {
		panic(err)
	}

	return result, structResult
}

// PrimaryStructLiteralExternal creates a value of the struct literal
// importName.structName{...}, as PrimaryStructLiteral does.
func PrimaryStructLiteralExternal(prgrm *ast.CXProgram, importName string, structName string, line int, column int, structFields []ast.CXExpression) []ast.CXExpression {
	name := generateTempVarName(prgrm, constants.LOCAL_PREFIX)
	writes, strct := primaryStructLiteralExternalWrites(prgrm, importName, structName, line, column, name, structFields)
	if strct == nil {
		return structLiteralInvalid(prgrm)
	}
	return structLiteralValue(prgrm, strct, name, writes)
}

// primaryStructLiteralExternalWrites is primaryStructLiteralWrites for a
// struct of the package importName.
func primaryStructLiteralExternalWrites(prgrm *ast.CXProgram, importName string, structName string, line int, column int, outputName stringpool.StringGid, structFields []ast.CXExpression) ([]ast.CXExpression, *ast.CXStruct) {
	var result []ast.CXExpression
	var structResult *ast.CXStruct
	var fieldNames []stringpool.StringGid
	structNameGid := prgrm.CXProgramNameIntern(structName)
	importNameGid := prgrm.CXProgramNameIntern(importName)
	if pkg, err := prgrm.GetCurrentPackage(); err == nil {
		if _, err := pkg.GetImport(prgrm, importNameGid); err == nil {
			if strct, err := prgrm.GetStruct(structNameGid, importNameGid); err == nil {
				structResult = strct
				for _, expr := range structFields {
					if expr.Type == ast.CX_LINE {
						continue
					}
					if expr.ExpressionType != ast.CXEXPR_STRUCT_LITERAL_FIELD {
						result = append(result, expr)
						continue
					}
					structLiteralFieldRepeatedCheck(prgrm, &fieldNames, expr)
					// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
					// Now an arena getter, site S1023.
					cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
					if err != nil {
						panic(err)
					}

					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S1024.
					cxAtomicOpOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(cxAtomicOp.GetOutputs(prgrm)[0])
					if cxAtomicOpOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
						cxAtomicOpOutputIdx := cxAtomicOpOutputTypeSig.Meta
						_, field := ast.MakeArgument(prgrm, stringpool.StringGidNil, CurrentFile, LineNo)
						argumentDebugPositionCopy(prgrm, field.ArgDetails, cxAtomicOpOutputTypeSig.ArgDetails)
						field.SetType(types.IDENTIFIER)
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1750.
						field.Name = prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Name
						structLiteralValueTypeSet(prgrm, strct, prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)), field.Name)

						expr.ExpressionType = ast.CXEXPR_STRUCT_LITERAL

						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1751.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Package = ast.CXPackageIndex(pkg.Index)
						// expr.ProgramOutput[0].Program = prgrm

						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1752.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).StructType = strct
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1753.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Size = strct.GetStructSize(prgrm)
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1754.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Name = outputName
						cxAtomicOpOutputTypeSig.Name = outputName

						fieldIdx := ast.CXArgumentIndex(field.Index)
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1755.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Fields = append(prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Fields, fieldIdx)
					} else if cxAtomicOpOutputTypeSig.Type == ast.TYPE_ATOMIC || cxAtomicOpOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
						panic("type signature is type atomic")
					} else {
						panic("type is not known")
					}

					result = append(result, expr)
				}
			} else if !prgrm.CXProgramImportFailed(importNameGid) {
				// A package whose import failed has no types, and its import is the error.
				structLiteralMissing(prgrm, line, column, err)
			}
		} else {
			panic(err)
		}
	} else {
		panic(err)
	}

	return result, structResult
}

// ArrayLiteralExpression creates an array literal expression.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 arraySizes - len(arraySizes) is the length of the array while contents of
//					 arraySizes define the size of each dimension.
//	 typeCode - type code of the array.
//	 line, column - where the literal starts, or a column of 0 for the token the
//					 parser is at, where its errors are.
//	 exprs - the array of expressions composing the array literal expression.
func ArrayLiteralExpression(prgrm *ast.CXProgram, arraySizes []types.Pointer, typeCode types.Code, line int, column int, exprs []ast.CXExpression) []ast.CXExpression {
	return arrayLiteralExpressionOf(prgrm, arraySizes, typeCode, nil, line, column, exprs)
}

// ArrayLiteralStructExpression handles an array literal of the struct
// structName, `[2]T{T{x: 1}, t}`, as ArrayLiteralExpression does for [2]i32.
func ArrayLiteralStructExpression(prgrm *ast.CXProgram, arraySizes []types.Pointer, structName string, line int, column int, exprs []ast.CXExpression) []ast.CXExpression {
	if column == 0 {
		line = LineNo
	}
	elt := DeclarationSpecifiersStruct(prgrm, structName, "", false, CurrentFile, line, column)
	if elt.StructType == nil {
		return structLiteralInvalid(prgrm)
	}
	return arrayLiteralExpressionOf(prgrm, arraySizes, types.STRUCT, elt.StructType, line, column, sliceLiteralStructElements(prgrm, elt.StructType, exprs, constants.ARRAY_ELEMENT_PREFIX))
}

// arrayLiteralExpressionOf is ArrayLiteralExpression for elements of type
// typeCode, or of the struct strct when it is set.
func arrayLiteralExpressionOf(prgrm *ast.CXProgram, arraySizes []types.Pointer, typeCode types.Code, strct *ast.CXStruct, line int, column int, exprs []ast.CXExpression) []ast.CXExpression {
	var result []ast.CXExpression

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	symName := generateTempVarName(prgrm, constants.LOCAL_PREFIX)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1027.
	arrVarExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1028.
	arrVarExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
	arrVarExprAtomicOpIdx := arrVarExpr.Index

	prgrm.CXAtomicOps[arrVarExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)
	_, arrVar := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
	sliceLiteralSetType(prgrm, arrVar, typeCode, strct, true)
	arrVar = DeclarationSpecifiers(arrVar, arraySizes, constants.DECL_ARRAY)
	arrVar.Package = ast.CXPackageIndex(pkg.Index)
	arrVar.PreviouslyDeclared = true

	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, arrVar)
	prgrm.CXAtomicOps[arrVarExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

	result = append(result, *arrVarExprCXLine, *arrVarExpr)

	var endPointsCounter int
	for _, expr := range exprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S1031.
		expression, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S1032.
		expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

		if expr.IsArrayLiteral() {
			expr.ExpressionType = ast.CXEXPR_UNUSED

			_, sym := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
			sliceLiteralSetType(prgrm, sym, typeCode, strct, false)
			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true

			idxArg := WritePrimary(prgrm, types.I32, encoder.Serialize(int32(endPointsCounter)), false)
			// index is always i32
			// hence, always an atomic type

			_, indexTypeSignature := prgrm.AllocCXTypeSignature()
			indexTypeSignature.Name = idxArg.Name
			indexTypeSignature.Type = ast.TYPE_ATOMIC
			indexTypeSignature.Meta = int(idxArg.Type)
			indexTypeSignature.Offset = idxArg.Offset // important for this
			indexTypeSignature.Package = idxArg.Package
			indexTypeSignature.ArgDetails = idxArg.ArgDetails
			// The signature keeps the debug record of the dropped literal argument.
			prgrm.ReleaseCXArg(ast.CXArgumentIndex(idxArg.Index))

			indexTypeSignatureIdx := indexTypeSignature.Index
			endPointsCounter++

			sym.Indexes = append(sym.Indexes, indexTypeSignatureIdx)
			sym.DereferenceOperations = append(sym.DereferenceOperations, constants.DEREF_ARRAY)
			sym.Lengths = arraySizes

			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S1036.
			symExprCXLine := MakeCXLineExpressionCurrent(prgrm)
			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S1037.
			symExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
			symExprAtomicOpIdx := symExpr.Index

			typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
			prgrm.CXAtomicOps[symExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

			if expressionOperator == nil {
				// then it's a literal
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now an arena insert, site S1039.
				opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_IDENTITY])
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Operator = opIdx
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs = expression.Outputs
			} else {
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Operator = expression.Operator
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs = expression.Inputs

				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
				// hack to get the correct lengths below
				// MakeAtomicOperatorExpression above may have grown CXAtomicOps, so refetch.
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S1041.
				expression, err = prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.AddOutput(prgrm, typeSigIdx)
			}
			prgrm.ReleaseCXArgUnlessNamed(sym, typeSigIdx)
			result = append(result, *symExprCXLine, *symExpr)
		} else {
			result = append(result, expr)
		}
	}

	if len(arraySizes) > 0 && types.Pointer(endPointsCounter) > arraySizes[0] {
		operands := ast.CompileErrorOperands{int64(arraySizes[0]), int64(endPointsCounter)}
		if column == 0 {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeArrayLiteralLength, CurrentFile, LineNo, operands)
		} else {
			prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeArrayLiteralLength, CurrentFile, line, column, operands)
		}
	}

	symNameOutput := generateTempVarName(prgrm, constants.LOCAL_PREFIX)

	_, symOutput := ast.MakeArgument(prgrm, symNameOutput, CurrentFile, LineNo)
	sliceLiteralSetType(prgrm, symOutput, typeCode, strct, true)
	symOutput = DeclarationSpecifiers(symOutput, arraySizes, constants.DECL_ARRAY)
	symOutput.Package = ast.CXPackageIndex(pkg.Index)
	symOutput.PreviouslyDeclared = true

	_, symInput := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
	sliceLiteralSetType(prgrm, symInput, typeCode, strct, false)
	symInput.Lengths = arraySizes
	symInput.Package = ast.CXPackageIndex(pkg.Index)
	symInput.PreviouslyDeclared = true

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1044.
	symExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1045.
	symExpr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
	symExprAtomicOpIdx := symExpr.Index
	prgrm.CXAtomicOps[symExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)

	typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symOutput)
	prgrm.CXAtomicOps[symExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

	typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symInput)
	prgrm.CXAtomicOps[symExprAtomicOpIdx].AddInput(prgrm, typeSigIdx)

	// symOutput.SynonymousTo = symInput.Name

	// marking the output so multidimensional arrays identify the expressions
	symExpr.ExpressionType = ast.CXEXPR_ARRAY_LITERAL
	result = append(result, *symExprCXLine, *symExpr)

	return result
}

// StructLiteralFields creates an initial expression for the
// struct literal expression.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	structName - name of the struct.
func StructLiteralFields(prgrm *ast.CXProgram, structName string, line int, column int) ast.CXExpression {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	_, arg := ast.MakeArgument(prgrm, stringpool.StringGidNil, CurrentFile, LineNo)
	// The errors of the field are at its name.
	details := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(arg.ArgDetails)
	details.FileLine, details.FileColumn = line, column
	arg.SetType(types.IDENTIFIER)
	arg.Name = prgrm.CXProgramNameIntern(structName)
	arg.Package = ast.CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1049.
	expr := ast.MakeAtomicOperatorExpression(prgrm, nil)
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1050.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, arg)
	expression.AddOutput(prgrm, typeSigIdx)

	expression.Package = ast.CXPackageIndex(pkg.Index)
	return *expr
}

// structLiteralValueTypeSet types a struct literal field value that an
// operator such as -1 leaves untyped with the declared type of the field.
func structLiteralValueTypeSet(prgrm *ast.CXProgram, strct *ast.CXStruct, value *ast.CXArgument, fieldName stringpool.StringGid) {
	if value.Type != types.UNDEFINED {
		return
	}
	if fieldSig, err := strct.GetField(prgrm, fieldName); err == nil {
		if fieldSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			value.Type = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fieldSig.Meta)).Type
		} else {
			value.Type = fieldSig.GetType(prgrm)
		}
	}
}

// StructLiteralField writes value to the field fieldName of a struct literal,
// marked so PrimaryStructLiteral can tell the write from the value. The name
// is at line and column.
func StructLiteralField(prgrm *ast.CXProgram, fieldName string, line int, column int, value []ast.CXExpression) []ast.CXExpression {
	value = ShortCircuitStrip(prgrm, value)
	// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
	// Now a slice of objects, sites S1482 S1483 S1484 S1485.
	field := []ast.CXExpression{StructLiteralFields(prgrm, fieldName, line, column)}
	var result []ast.CXExpression
	if structLiteralReference(value) {
		result = StructLiteralAssignment(prgrm, field, value)
	} else {
		result = Assignment(prgrm, field, "=", value)
	}
	for i := len(result) - 1; i >= 0; i-- {
		if result[i].Type != ast.CX_LINE {
			result[i].ExpressionType = ast.CXEXPR_STRUCT_LITERAL_FIELD
			break
		}
	}
	return result
}

// structLiteralReference reports whether value is &T{...}, the only struct
// literal that is not a value.
func structLiteralReference(value []ast.CXExpression) bool {
	for i := range value {
		if value[i].Type != ast.CX_LINE {
			return value[i].IsStructLiteral()
		}
	}
	return false
}

// structLiteralVariable is a signature of the struct variable name, the
// declaration when declaration is set.
func structLiteralVariable(prgrm *ast.CXProgram, strct *ast.CXStruct, name stringpool.StringGid, declaration bool) ast.CXTypeSignatureIndex {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	_, arg := ast.MakeArgument(prgrm, name, CurrentFile, LineNo)
	sliceLiteralSetType(prgrm, arg, types.STRUCT, strct, declaration)
	arg.Package = ast.CXPackageIndex(pkg.Index)
	arg.PreviouslyDeclared = true
	// A named signature lets the symbols table take the declaration as a temp.
	return ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, arg)
}

// structLiteralValue declares the variable name, which zeroes it, then runs
// writes and ends with an expression that gives the variable.
func structLiteralValue(prgrm *ast.CXProgram, strct *ast.CXStruct, name stringpool.StringGid, writes []ast.CXExpression) []ast.CXExpression {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	declCXLine := MakeCXLineExpressionCurrent(prgrm)
	decl := ast.MakeAtomicOperatorExpression(prgrm, nil)
	prgrm.CXAtomicOps[decl.Index].Package = ast.CXPackageIndex(pkg.Index)
	prgrm.CXAtomicOps[decl.Index].AddOutput(prgrm, structLiteralVariable(prgrm, strct, name, true))
	result := []ast.CXExpression{*declCXLine, *decl}

	for i := range writes {
		if writes[i].Type != ast.CX_LINE && writes[i].IsStructLiteral() {
			sig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[writes[i].Index].GetOutputs(prgrm)[0])
			out := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta))
			if out.Name == name {
				out.DereferenceOperations = append(out.DereferenceOperations, constants.DEREF_FIELD)
			}
		}
		result = append(result, writes[i])
	}

	use := ast.MakeAtomicOperatorExpression(prgrm, nil)
	prgrm.CXAtomicOps[use.Index].Package = ast.CXPackageIndex(pkg.Index)
	prgrm.CXAtomicOps[use.Index].AddOutput(prgrm, structLiteralVariable(prgrm, strct, name, false))
	return append(result, *use)
}

// structLiteralFieldRepeatedCheck reports the field write expr when an
// earlier write of the same struct literal named its field.
func structLiteralFieldRepeatedCheck(prgrm *ast.CXProgram, fieldNames *[]stringpool.StringGid, expr ast.CXExpression) {
	name := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[expr.Index].GetOutputs(prgrm)[0]).Name
	for _, seen := range *fieldNames {
		if seen == name {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeStructLiteralFieldRepeated, prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[expr.Index].GetOutputs(prgrm)[0]).ArgDetails, ast.CompileErrorOperands{int64(name)})
			return
		}
	}
	*fieldNames = append(*fieldNames, name)
}

// sliceLiteralStructInner reports whether element is a slice, such as a
// []T literal, an element of [][]T{...} that the outer [] gives its
// extra dimension. The append of the element checks its type.
func sliceLiteralStructInner(prgrm *ast.CXProgram, element []ast.CXExpression) bool {
	last := len(element) - 1
	for last > 0 && element[last].Type == ast.CX_LINE {
		last--
	}
	if last < 0 || len(prgrm.CXAtomicOps[element[last].Index].GetOutputs(prgrm)) == 0 {
		return false
	}
	sig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[element[last].Index].GetOutputs(prgrm)[0])
	switch sig.Type {
	case ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		return true
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		return hasDeclSpec(prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta)), constants.DECL_SLICE)
	}
	return false
}

// PrimaryStructLiteralExternalOf creates pkg.T{...}, where pkgExprs is the
// expression pkg, as PrimaryStructLiteralExternal does.
func PrimaryStructLiteralExternalOf(prgrm *ast.CXProgram, pkgExprs []ast.CXExpression, structName string, line int, column int, structFields []ast.CXExpression) []ast.CXExpression {
	cxAtomicOp, err := prgrm.GetCXAtomicOpFromExpressions(pkgExprs, 0)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S1492.
	importName := prgrm.GetCXTypeSignatureFromArray(cxAtomicOp.GetOutputs(prgrm)[0]).Name
	return PrimaryStructLiteralExternal(prgrm, prgrm.CXProgramNameText(importName), structName, line, column, structFields)
}

// structLiteralInvalid is the value of a literal of a struct that does not
// exist, whose error is recorded. Its uses report nothing more.
func structLiteralInvalid(prgrm *ast.CXProgram) []ast.CXExpression {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	name := generateTempVarName(prgrm, constants.LOCAL_PREFIX)
	variable := func(declaration bool) ast.CXTypeSignatureIndex {
		_, arg := ast.MakeArgument(prgrm, name, CurrentFile, LineNo)
		arg.SetType(types.INVALID)
		arg.Package = ast.CXPackageIndex(pkg.Index)
		arg.PreviouslyDeclared = !declaration
		return ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, arg)
	}
	declCXLine := MakeCXLineExpressionCurrent(prgrm)
	decl := ast.MakeAtomicOperatorExpression(prgrm, nil)
	prgrm.CXAtomicOps[decl.Index].Package = ast.CXPackageIndex(pkg.Index)
	prgrm.CXAtomicOps[decl.Index].AddOutput(prgrm, variable(true))
	use := ast.MakeAtomicOperatorExpression(prgrm, nil)
	prgrm.CXAtomicOps[use.Index].Package = ast.CXPackageIndex(pkg.Index)
	prgrm.CXAtomicOps[use.Index].AddOutput(prgrm, variable(false))
	return []ast.CXExpression{*declCXLine, *decl, *use}
}

// structLiteralMissing records err, the struct of a literal that does not
// exist, at line and column, where its name is, or at the token the parser
// is at for a column of 0.
func structLiteralMissing(prgrm *ast.CXProgram, line int, column int, err error) {
	operands := ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())}
	if column == 0 {
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeMessage, CurrentFile, LineNo, operands)
		return
	}
	prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeMessage, CurrentFile, line, column, operands)
}

// argumentDebugPositionCopy gives the debug record details the position of
// the debug record from.
func argumentDebugPositionCopy(prgrm *ast.CXProgram, details ast.CXArgumentDebugGid, from ast.CXArgumentDebugGid) {
	if details == ast.CXArgumentDebugGidNil || from == ast.CXArgumentDebugGidNil {
		return
	}
	file, line, column := ast.CXArgumentDebugPosition(prgrm, from)
	record := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(details)
	record.FileName, record.FileLine, record.FileColumn = file, line, column
}
