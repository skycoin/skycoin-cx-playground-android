package actions

import (
	"strings"

	cxpackages "github.com/skycoin/skycoin-cx/t0-packages" // thin: only IsDefaultPackage, SysPrefix; no depth gain
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// PostfixExpressionArray handles the postfix expression arrays.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		prevExprs - the previous expressions that compose the postfix expression.
//	 postExprs - the array expressions.
func PostfixExpressionArray(prgrm *ast.CXProgram, prevExprs []ast.CXExpression, postExprs []ast.CXExpression) []ast.CXExpression {
	prevExprs = MethodCallValue(prgrm, prevExprs)
	var elt *ast.CXArgument

	prevExpressionIdx := prevExprs[len(prevExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1094.
	prevExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[prevExpressionIdx].Operator)
	prevExprCXLine, _ := prgrm.GetPreviousCXLine(prevExprs, len(prevExprs)-1)

	if prevExpressionOperator != nil && len(prgrm.CXAtomicOps[prevExpressionIdx].GetOutputs(prgrm)) == 0 {
		genName := generateTempVarName(prgrm, constants.LOCAL_PREFIX)

		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S1095.
		useExprCXLine := ast.MakeCXLineExpression(prgrm, prevExprCXLine.FileName, prevExprCXLine.LineNumber, prevExprCXLine.LineStr)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S1096.
		useExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
		useExpressionIdx := useExpr.Index
		prgrm.CXAtomicOps[useExpressionIdx].Package = prgrm.CXAtomicOps[prevExpressionIdx].Package

		prevExpressionOperatorOutputs := prevExpressionOperator.GetOutputs(prgrm)
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1097.
		prevExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prevExpressionOperatorOutputs[0])
		if prevExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1098.
			prevExpressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(prevExpressionOperatorOutputTypeSig.Meta))

			prevExpressionOperatorOutput := prevExpressionOperatorOutputArg
			_, out := ast.MakeArgument(prgrm, genName, prevExprCXLine.FileName, prevExprCXLine.LineNumber-1)
			out.SetType(prevExpressionOperatorOutput.Type)

			out.DeclarationSpecifiers = prevExpressionOperatorOutput.DeclarationSpecifiers
			out.StructType = prevExpressionOperatorOutput.StructType
			out.Size = prevExpressionOperatorOutput.Size
			out.Lengths = prevExpressionOperatorOutput.Lengths
			out.PreviouslyDeclared = true
			out.Package = prevExpressionOperatorOutput.Package

			typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
			prgrm.CXAtomicOps[prevExpressionIdx].AddOutput(prgrm, typeSigIdx)

			_, inp := ast.MakeArgument(prgrm, genName, prevExprCXLine.FileName, prevExprCXLine.LineNumber)
			inp.SetType(prevExpressionOperatorOutput.Type)

			inp.DeclarationSpecifiers = prevExpressionOperatorOutput.DeclarationSpecifiers
			inp.StructType = prevExpressionOperatorOutput.StructType
			inp.Size = prevExpressionOperatorOutput.Size
			inp.Lengths = prevExpressionOperatorOutput.Lengths
			inp.PreviouslyDeclared = true

			if ast.IsTypeArrayAtomic(inp) {
				typeSigIdx = ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, inp)
			} else {
				typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, inp)
			}
			prgrm.CXAtomicOps[useExpressionIdx].AddOutput(prgrm, typeSigIdx)
		} else if prevExpressionOperatorOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S1103.
			sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(prevExpressionOperatorOutputTypeSig.Meta)

			newTypeSigSliceGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
			newTypeSigSlice := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(newTypeSigSliceGid)
			newTypeSigSlice.Meta = sliceDetails.Meta
			newTypeSigSlice.Lengths = sliceDetails.Lengths
			newTypeSigSliceIdx := int(objmodel.GidIndex(int64(newTypeSigSliceGid)))

			argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
			argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
			argDetails.FileName = prevExprCXLine.FileName
			argDetails.FileLine = prevExprCXLine.LineNumber - 1
			argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
			_, newTypeSig := prgrm.AllocCXTypeSignature()
			newTypeSig.ArgDetails = argDetailsGid
			newTypeSig.Name = genName
			newTypeSig.Package = prevExpressionOperatorOutputTypeSig.Package
			newTypeSig.Type = prevExpressionOperatorOutputTypeSig.Type
			newTypeSig.Meta = newTypeSigSliceIdx

			outNewTypeSigIdx := newTypeSig.Index
			prgrm.CXAtomicOps[prevExpressionIdx].AddOutput(prgrm, outNewTypeSigIdx)

			inpNewTypeSigSliceGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
			inpNewTypeSigSlice := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(inpNewTypeSigSliceGid)
			inpNewTypeSigSlice.Meta = sliceDetails.Meta
			inpNewTypeSigSlice.Lengths = sliceDetails.Lengths
			inpNewTypeSigSliceIdx := int(objmodel.GidIndex(int64(inpNewTypeSigSliceGid)))

			inpArgDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
			inpArgDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(inpArgDetailsGid)
			inpArgDetails.FileName = prevExprCXLine.FileName
			inpArgDetails.FileLine = prevExprCXLine.LineNumber - 1
			inpArgDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(inpArgDetails.FileName, inpArgDetails.FileLine)
			_, inpNewTypeSig := prgrm.AllocCXTypeSignature()
			inpNewTypeSig.ArgDetails = inpArgDetailsGid
			inpNewTypeSig.Name = genName
			inpNewTypeSig.Package = prevExpressionOperatorOutputTypeSig.Package
			inpNewTypeSig.Type = prevExpressionOperatorOutputTypeSig.Type
			inpNewTypeSig.Meta = inpNewTypeSigSliceIdx
			inpNewTypeSigIdx := inpNewTypeSig.Index
			prgrm.CXAtomicOps[useExpressionIdx].AddOutput(prgrm, inpNewTypeSigIdx)
		} else {
			panic("type is not known\n")
		}

		prevExprs = append(prevExprs, *useExprCXLine, *useExpr)
	}

	prevExpression, err := prgrm.GetCXAtomicOpFromExpressions(prevExprs, len(prevExprs)-1)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S1114.
	prevExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prevExpression.GetOutputs(prgrm)[0])

	// nil for a slice output, which has no fields.
	var prevExpressionOutputArg *ast.CXArgument
	if prevExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1116.
		prevExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(prevExpressionOutputTypeSig.Meta))

		if len(prevExpressionOutputArg.Fields) > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1117.
			elt = prgrm.GetCXArgFromArray(prevExpressionOutputArg.Fields[len(prevExpressionOutputArg.Fields)-1])
		} else {
			elt = prevExpressionOutputArg
		}

		// elt.IsArray = false
		elt.DereferenceOperations = append(elt.DereferenceOperations, constants.DEREF_ARRAY)
		elt.DeclarationSpecifiers = append(elt.DeclarationSpecifiers, constants.DECL_INDEXING)

	} else if prevExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || prevExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
		// TODO: implement correct way when it is possible
		// temporarily return back to cx arg deprecate
		_, arg := ast.MakeArgument(prgrm, prevExpressionOutputTypeSig.Name, CurrentFile, LineNo) // fix: line numbers in errors sometimes report +1 or -1. Issue #195
		arg.SetType(types.IDENTIFIER)
		arg.Name = prevExpressionOutputTypeSig.Name
		arg.Package = prevExpressionOutputTypeSig.Package
		arg.PassBy = prevExpressionOutputTypeSig.PassBy

		argIdx := ast.CXArgumentIndex(arg.Index)
		prevExpressionOutputTypeSig.Type = ast.TYPE_CXARGUMENT_DEPRECATE
		prevExpressionOutputTypeSig.Meta = int(argIdx)

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1120.
		prevExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(prevExpressionOutputTypeSig.Meta))

		elt = prevExpressionOutputArg
		// elt.IsArray = false
		elt.DereferenceOperations = append(elt.DereferenceOperations, constants.DEREF_ARRAY)
		elt.DeclarationSpecifiers = append(elt.DeclarationSpecifiers, constants.DECL_INDEXING)
	} else if prevExpressionOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
		prevExpressionOutputTypeSig.IsDeref = true
	} else {
		panic("type is not known")
	}

	postExpressionIdx := postExprs[len(postExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1123.
	postExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[postExpressionIdx].Operator)
	postExpressionOperatorOutputs := postExpressionOperator.GetOutputs(prgrm)

	if prevExpressionOutputArg != nil && len(prevExpressionOutputArg.Fields) > 0 {
		fldIdx := prevExpressionOutputArg.Fields[len(prevExpressionOutputArg.Fields)-1]

		if postExpressionOperator == nil {
			postExpressionOutputIndex := prgrm.CXAtomicOps[postExpressionIdx].GetOutputs(prgrm)[0]
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1756.
			prgrm.GetCXArg(ast.CXArgumentIndex(fldIdx)).Indexes = append(prgrm.GetCXArg(ast.CXArgumentIndex(fldIdx)).Indexes, postExpressionOutputIndex)
		} else {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1124.
			postExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(postExpressionOperatorOutputs[0])
			var postExpressionOperatorOutputArg *ast.CXArgument
			if postExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1126.
				postExpressionOperatorOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(postExpressionOperatorOutputTypeSig.Meta))
			} else {
				panic("type is not cx argument deprecate\n\n")
			}

			_, sym := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
			sym.SetType(postExpressionOperatorOutputArg.Type)
			sym.Package = prgrm.CXAtomicOps[postExpressionIdx].Package
			sym.PreviouslyDeclared = true

			typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, sym)
			prgrm.CXAtomicOps[postExpressionIdx].AddOutput(prgrm, typeSigIdx)

			prevExprs = append(postExprs, prevExprs...)

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1757.
			prgrm.GetCXArg(ast.CXArgumentIndex(fldIdx)).Indexes = append(prgrm.GetCXArg(ast.CXArgumentIndex(fldIdx)).Indexes, typeSigIdx)
		}
	} else {
		if len(prgrm.CXAtomicOps[postExpressionIdx].GetOutputs(prgrm)) < 1 {
			// then it's an expression (e.g. i32.add(0, 0))
			// we create a gensym for it

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1129.
			postExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(postExpressionOperatorOutputs[0])
			var typeSigIdx ast.CXTypeSignatureIndex
			if postExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1130.
				postExpressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(postExpressionOperatorOutputTypeSig.Meta))
				_, idxSym := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
				idxSym.SetType(postExpressionOperatorOutputArg.Type)
				idxSym.Size = postExpressionOperatorOutputArg.Size

				idxSym.Package = prgrm.CXAtomicOps[postExpressionIdx].Package
				idxSym.PreviouslyDeclared = true

				typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, idxSym)
			} else if postExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
				argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
				argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
				argDetails.FileName = CurrentFile
				argDetails.FileLine = LineNo
				argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
				_, newTypeSig := prgrm.AllocCXTypeSignature()
				newTypeSig.ArgDetails = argDetailsGid
				newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
				newTypeSig.Package = prgrm.CXAtomicOps[postExpressionIdx].Package
				newTypeSig.Type = postExpressionOperatorOutputTypeSig.Type
				newTypeSig.Meta = postExpressionOperatorOutputTypeSig.Meta
				newTypeSig.Offset = postExpressionOperatorOutputTypeSig.Offset
				typeSigIdx = newTypeSig.Index
			} else if postExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
				argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
				argDetails.FileName = CurrentFile
				argDetails.FileLine = LineNo
				argDetails.FileColumn = prgrm.CXProgramCompileErrorColumn(argDetails.FileName, argDetails.FileLine)
				_, newTypeSig := prgrm.AllocCXTypeSignature()
				newTypeSig.ArgDetails = argDetailsGid
				newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
				newTypeSig.Package = prgrm.CXAtomicOps[postExpressionIdx].Package
				newTypeSig.Type = postExpressionOperatorOutputTypeSig.Type
				newTypeSig.Meta = postExpressionOperatorOutputTypeSig.Meta
				newTypeSig.Offset = postExpressionOperatorOutputTypeSig.Offset
				typeSigIdx = newTypeSig.Index
			} else {
				panic("type is not known")
			}

			prgrm.CXAtomicOps[postExpressionIdx].AddOutput(prgrm, typeSigIdx)

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1139.
			prevExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prevExpression.GetOutputs(prgrm)[0])
			var prevExpressionOutputArg *ast.CXArgument
			if prevExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1141.
				prevExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(prevExpressionOutputTypeSig.Meta))
			} else {
				panic("type is not cx argument deprecate\n\n")
			}

			prevExpressionOutput := prevExpressionOutputArg
			prevExpressionOutput.Indexes = append(prevExpressionOutput.Indexes, typeSigIdx)

			// we push the index expression
			prevExprs = append(postExprs, prevExprs...)
		} else {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1142.
			prevOutsTypeSig := prgrm.GetCXTypeSignatureFromArray(prevExpression.GetOutputs(prgrm)[0])
			if prevOutsTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				prevOutsIdx := ast.CXArgumentIndex(prevOutsTypeSig.Meta)

				postOutsIndex := prgrm.CXAtomicOps[postExpressionIdx].GetOutputs(prgrm)[0]
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1758.
				prgrm.GetCXArg(ast.CXArgumentIndex(prevOutsIdx)).Indexes = append(prgrm.GetCXArg(ast.CXArgumentIndex(prevOutsIdx)).Indexes, postOutsIndex)
			} else if prevOutsTypeSig.Type == ast.TYPE_ATOMIC || prevOutsTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				panic("type is not cx argument deprecate\n\n")
			} else if prevOutsTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S1143.
				sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(prevOutsTypeSig.Meta)

				postOutsIndex := prgrm.CXAtomicOps[postExpressionIdx].GetOutputs(prgrm)[0]
				sliceDetails.Indexes = append(sliceDetails.Indexes, postOutsIndex)
			} else {
				panic("type is not known")
			}

		}
	}

	// An index read after a method call keeps the call.
	if postExpressionOperator == nil && len(postExprs) > 1 {
		prevExprs = append(append([]ast.CXExpression{}, postExprs[:len(postExprs)-1]...), prevExprs...)
	}

	return prevExprs
}

// PostfixExpressionNative handles native postfix expression function calls.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		typeCode - type code of the native expression.
//	 opStrCode - name of the function to be called that belongs to the native
//					expression.
func PostfixExpressionNative(prgrm *ast.CXProgram, typeCode types.Code, opStrCode string) []ast.CXExpression {
	// these will always be native functions
	native, ok := prgrm.CXProgramNativeFind(&ast.NativeNames, prgrm.CXProgramNameIntern(typeCode.Name()), stringpool.StringGidNil, prgrm.CXProgramNameIntern(opStrCode))
	opName := ""
	if !ok {
		opName = typeCode.Name() + "." + opStrCode
		if !renamedOpcodeRecord(prgrm, CurrentFile, LineNo, opName) {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeFunctionMissing, CurrentFile, LineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(opName)})
		}
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1144.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1145.
	var expr *ast.CXExpression
	if ok {
		expr = ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[native.NativeNameEntryCode])
	} else {
		// The call of a missing native is typed by a stand in, see invalidFunctionIndex.
		expr = ast.MakeAtomicOperatorExpression(prgrm, nil)
	}
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1146.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	expression.Package = ast.CXPackageIndex(pkg.Index)
	if !ok {
		expression.Operator = invalidFunctionIndex(prgrm, expression.Package, prgrm.CXProgramNameIntern(opName))
	}
	// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
	// Now a slice of objects, site S1147.
	return []ast.CXExpression{*exprCXLine, *expr}
}

// PostfixExpressionEmptyFunCall handles postfix expression empty function calls
// or the function calls that doesn't have any input args.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	prevExprs - the previous expressions that compose the postfix expression.
func PostfixExpressionEmptyFunCall(prgrm *ast.CXProgram, prevExprs []ast.CXExpression) []ast.CXExpression {
	if functionCallRepeated(prgrm, prevExprs) {
		return prevExprs
	}
	prevExpression, err := prgrm.GetCXAtomicOpFromExpressions(prevExprs, len(prevExprs)-1)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1148.
	prevExpressionOperator := prgrm.GetFunctionFromArray(prevExpression.Operator)

	var firstPrevExpressionIdx int
	for i := 0; i < len(prevExprs); i++ {
		if prevExprs[i].Type == ast.CX_ATOMIC_OPERATOR {
			firstPrevExpressionIdx = prevExprs[i].Index
			break
		}
	}

	// nil when there is no output or it is not an argument.
	var prevExpressionOutputArg *ast.CXArgument
	if prevExpression.Outputs != nil {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1150.
		prevExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prevExpression.GetOutputs(prgrm)[0])
		if prevExpression.Outputs != nil {
			if prevExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1151.
				prevExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(prevExpressionOutputTypeSig.Meta))
			}
		}
	}

	if prevExpressionOutputArg != nil && len(prevExpressionOutputArg.Fields) > 0 {
		// then it's a method call or function in field

	} else if prevExpressionOperator == nil {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1153.
		prevExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prevExpression.GetOutputs(prgrm)[0])
		if native, ok := prgrm.CXProgramNativeFind(&ast.NativeNames, stringpool.StringGidNil, stringpool.StringGidNil, prevExpressionOutputTypeSig.Name); ok && !functionDeclared(prgrm, prevExpressionOutputTypeSig) {
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				prgrm.CXAtomicOps[firstPrevExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)
			}
			prgrm.CXAtomicOps[firstPrevExpressionIdx].Outputs = nil
			// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
			// Now an arena insert, site S1154.
			opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[native.NativeNameEntryCode])
			prgrm.CXAtomicOps[firstPrevExpressionIdx].Operator = opIdx
		}

		prgrm.CXAtomicOps[firstPrevExpressionIdx].Inputs = nil
	}

	return FunctionCall(prgrm, prevExprs, nil)
}

// PostfixExpressionFunCall handles postfix expression function calls.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		prevExprs - the previous expressions that compose the postfix expression.
//	 args - input args list for the function call.
func PostfixExpressionFunCall(prgrm *ast.CXProgram, prevExprs []ast.CXExpression, args [][]ast.CXExpression) []ast.CXExpression {
	// An unresolved callee has already been reported as a compile error.
	if len(prevExprs) == 0 {
		return nil
	}
	if functionCallRepeated(prgrm, prevExprs) {
		return prevExprs
	}

	lastPrevExpression, err := prgrm.GetCXAtomicOpFromExpressions(prevExprs, len(prevExprs)-1)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1155.
	lastPrevExpressionOperator := prgrm.GetFunctionFromArray(lastPrevExpression.Operator)

	firstPrevExpressionIdx := prevExprs[0].Index

	var lastPrevExpressionOutputTypeSig *ast.CXTypeSignature

	// nil when there is no output or it is atomic.
	var lastPrevExpressionOutputArg *ast.CXArgument
	if lastPrevExpression.Outputs != nil {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1157.
		lastPrevExpressionOutputTypeSig = prgrm.GetCXTypeSignatureFromArray(lastPrevExpression.GetOutputs(prgrm)[0])

		if lastPrevExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1158.
			lastPrevExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastPrevExpressionOutputTypeSig.Meta))
		} else if lastPrevExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || lastPrevExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// do nothing
		} else {
			panic("type is not known")
		}
	}

	if lastPrevExpressionOutputArg != nil && len(lastPrevExpressionOutputArg.Fields) > 0 {
		// then it's a method
		// prevExprs[len(prevExprs) - 1].IsMethodCall = true

	} else if lastPrevExpressionOperator == nil {
		if native, ok := prgrm.CXProgramNativeFind(&ast.NativeNames, stringpool.StringGidNil, stringpool.StringGidNil, lastPrevExpressionOutputTypeSig.Name); ok && !functionDeclared(prgrm, lastPrevExpressionOutputTypeSig) {
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				prgrm.CXAtomicOps[firstPrevExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)
			}
			prgrm.CXAtomicOps[firstPrevExpressionIdx].Outputs = nil
			// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
			// Now an arena insert, site S1159.
			opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[native.NativeNameEntryCode])
			prgrm.CXAtomicOps[firstPrevExpressionIdx].Operator = opIdx
		}

		prgrm.CXAtomicOps[firstPrevExpressionIdx].Inputs = nil
	}

	return FunctionCall(prgrm, prevExprs, args)
}

// PostfixExpressionIncDec handles the incrementing or decrementing of the
// postfix expression.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		prevExprs - the previous expressions that compose the postfix expression.
//	 isInc - true if expression is to be incremented, false if to be decremented.
func PostfixExpressionIncDec(prgrm *ast.CXProgram, prevExprs []ast.CXExpression, isInc bool) []ast.CXExpression {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1160.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	var expr *ast.CXExpression
	if isInc {
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S1161.
		expr = ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_ADD])
	} else {
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S1162.
		expr = ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_SUB])
	}

	lastPrevExpression, err := prgrm.GetCXAtomicOpFromExpressions(prevExprs, len(prevExprs)-1)
	if err != nil {
		panic(err)
	}

	expressionIdx := expr.Index
	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	typeSigIdx := lastPrevExpression.GetOutputs(prgrm)[0]
	prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, ArgumentReadCopy(prgrm, typeSigIdx))

	// incDecOperandAdd adds the 1 once the type of the operand is known.
	prgrm.CXAtomicOps[expressionIdx].AddOutput(prgrm, typeSigIdx)

	// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
	// Now a slice of objects, site S1164.
	exprs := append([]ast.CXExpression{}, prevExprs[:len(prevExprs)-1]...)
	return append(exprs, *exprCXLine, *expr)
}

// PostfixExpressionField handles the dot notation that can be followed by an identifier.
// Examples are: `foo.bar`, `foo().bar`, `pkg.foo`
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		prevExprs - the previous expressions that compose the left expression
//					i.e. 'foo' in the example above.
//	 ident - field name or the identifier after the dot notation.
func PostfixExpressionField(prgrm *ast.CXProgram, prevExprs []ast.CXExpression, ident string) []ast.CXExpression {
	prevExprs = MethodCallValue(prgrm, prevExprs)
	lastExpr := prevExprs[len(prevExprs)-1]

	lastExpressionIdx := lastExpr.Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1165.
	lastExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastExpressionIdx].Operator)

	lastExprCXLine, _ := prgrm.GetPreviousCXLine(prevExprs, len(prevExprs)-1)

	// Then it's a function call, e.g. foo().fld
	// and we need to create some auxiliary variables to hold the result from
	// the function call
	if lastExpressionOperator != nil {
		lastExpressionOperatorOutputs := lastExpressionOperator.GetOutputs(prgrm)
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1166.
		lastExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastExpressionOperatorOutputs[0])

		symName := generateTempVarName(prgrm, constants.LOCAL_PREFIX)
		if sigType := lastExpressionOperatorOutputTypeSig.Type; sigType == ast.TYPE_CXARGUMENT_DEPRECATE || sigType == ast.TYPE_STRUCT || sigType == ast.TYPE_POINTER_STRUCT {
			opOut := postfixCallResultArgument(prgrm, lastExpressionOperatorOutputTypeSig)

			// we associate the result of the function call to the aux variable
			_, out := ast.MakeArgument(prgrm, symName, lastExprCXLine.FileName, lastExprCXLine.LineNumber)
			out.SetType(opOut.Type)
			out.DeclarationSpecifiers = opOut.DeclarationSpecifiers
			out.StructType = opOut.StructType
			out.Size = opOut.Size
			// out.IsArray = opOut.IsArray
			// out.IsReference = opOut.IsReference
			out.Lengths = opOut.Lengths
			out.Package = prgrm.CXAtomicOps[lastExpressionIdx].Package
			out.PreviouslyDeclared = true

			typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
			prgrm.CXAtomicOps[lastExpressionIdx].AddOutput(prgrm, typeSigIdx)

			// we need to create an expression to hold all the modifications
			// that will take place after this if statement
			_, inp := ast.MakeArgument(prgrm, symName, lastExprCXLine.FileName, lastExprCXLine.LineNumber)
			inp.SetType(opOut.Type)
			inp.DeclarationSpecifiers = opOut.DeclarationSpecifiers
			inp.StructType = opOut.StructType
			inp.Size = opOut.Size
			inp.Package = prgrm.CXAtomicOps[lastExpressionIdx].Package

			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S1171.
			exprCXLine := ast.MakeCXLineExpression(prgrm, lastExprCXLine.FileName, lastExprCXLine.LineNumber, lastExprCXLine.LineStr)
			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S1172.
			expr := ast.MakeAtomicOperatorExpression(prgrm, nil)

			// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
			// Now an arena getter, site S1173.
			expression, err := prgrm.GetCXAtomicOp(expr.Index)
			if err != nil {
				panic(err)
			}

			expression.Package = prgrm.CXAtomicOps[lastExpressionIdx].Package
			// A named signature keeps the fields and method calls of f().x on inp.
			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, inp)
			expression.AddOutput(prgrm, typeSigIdx)
			prevExprs = append(prevExprs, *exprCXLine, *expr)

			lastExpr = prevExprs[len(prevExprs)-1]

			lastExpressionIdx = lastExpr.Index
		} else {
			panic("type is not cx argument deprecate\n\n")
		}

	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S1187.
	leftExprOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastExpressionIdx].GetOutputs(prgrm)[0])

	// then left is a first (e.g first.rest) and right is a rest
	// let's check if left is a package
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	identGid := prgrm.CXProgramNameIntern(ident)
	if imp, err := pkg.GetImport(prgrm, leftExprOutputTypeSig.Name); err == nil {
		// the external property will be propagated to the following arguments
		// this way we avoid considering these arguments as module names

		if cxpackages.IsDefaultPackage(prgrm.CXProgramNameText(leftExprOutputTypeSig.Name)) {
			if constantName, ok := prgrm.CXProgramNativeFind(&ConstNames, leftExprOutputTypeSig.Name, stringpool.StringGidNil, identGid); ok {
				constant := Constants[constantName.NativeNameEntryCode]
				valArg := WritePrimary(prgrm, constant.Type, constant.Value, false)

				// TODO: to be removed and implemented with correct process to change the index
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S1188.
				lastExpressionOutputFieldTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastExpressionIdx].Outputs.Fields[0])
				if lastExpressionOutputFieldTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					lastExpressionOutputFieldTypeSig.Meta = valArg.Index
					lastExpressionOutputFieldTypeSig.Name = valArg.Name
					lastExpressionOutputFieldTypeSig.Offset = valArg.Offset
				} else if lastExpressionOutputFieldTypeSig.Type == ast.TYPE_ATOMIC {
					// TODO: Review if this is the correct one
					typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, valArg)
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S1778.
					prgrm.GetCXTypeSignatureFromArray(typeSigIdx).Offset = valArg.Offset
					prgrm.ReleaseCXArgUnlessNamed(valArg, typeSigIdx)

					prgrm.CXAtomicOps[lastExpressionIdx].Outputs.Fields[0] = typeSigIdx
				} else if lastExpressionOutputFieldTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					// TODO: Review if this is the correct one
					typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, valArg)
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S1779.
					prgrm.GetCXTypeSignatureFromArray(typeSigIdx).Offset = valArg.Offset
					prgrm.ReleaseCXArgUnlessNamed(valArg, typeSigIdx)

					prgrm.CXAtomicOps[lastExpressionIdx].Outputs.Fields[0] = typeSigIdx
				} else {
					panic("type is not known")
				}

				return prevExprs
			} else if native, ok := prgrm.CXProgramNativeFind(&ast.NativeNames, leftExprOutputTypeSig.Name, stringpool.StringGidNil, identGid); ok {
				// then it's a native
				// TODO: we'd be referring to the function itself, not a function call
				// (functions as first-class objects)
				leftExprOutputTypeSigName := prgrm.CXProgramNameIntern(native.NativeNameEntryText)
				leftExprOutputTypeSig.Name = leftExprOutputTypeSigName
				if leftExprOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					leftExprIdx := leftExprOutputTypeSig.Meta
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1759.
					prgrm.GetCXArg(ast.CXArgumentIndex(leftExprIdx)).Name = leftExprOutputTypeSigName
				}

				return prevExprs
			}
		}

		leftExprOutputTypeSig.Package = ast.CXPackageIndex(imp.Index)
		if leftExprOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			leftExprIdx := leftExprOutputTypeSig.Meta
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1760.
			prgrm.GetCXArg(ast.CXArgumentIndex(leftExprIdx)).Package = ast.CXPackageIndex(imp.Index)
		}

		if glbl, err := imp.GetGlobal(prgrm, identGid); err == nil {
			// then it's a global
			// prevExprs[len(prevExprs)-1].ProgramOutput[0] = glbl
			if glbl.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1191.
				glblArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(glbl.Meta))

				// then it's a global
				// prevExprs[len(prevExprs)-1].ProgramOutput[0] = glbl
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S1192.
				lastExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastExpressionIdx].GetOutputs(prgrm)[0])

				var lastExpressionOutputArg *ast.CXArgument
				if lastExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1194.
					lastExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastExpressionOutputTypeSig.Meta))
					lastExpressionOutputArg.Name = glblArg.Name
					lastExpressionOutputTypeSig.Name = glblArg.Name

					lastExpressionOutputArg.Type = glblArg.Type
					lastExpressionOutputArg.StructType = glblArg.StructType
					lastExpressionOutputArg.Size = glblArg.Size
					lastExpressionOutputArg.PointerTargetType = glblArg.PointerTargetType
					lastExpressionOutputArg.Package = glblArg.Package
				} else if lastExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || lastExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					lastExpressionOutputTypeSig.Name = glbl.Name
					lastExpressionOutputTypeSig.Type = glbl.Type
					lastExpressionOutputTypeSig.Meta = glbl.Meta
					lastExpressionOutputTypeSig.Offset = glbl.Offset
				} else {
					panic("type is not known")
				}

			} else if glbl.Type == ast.TYPE_ATOMIC || glbl.Type == ast.TYPE_POINTER_ATOMIC {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S1195.
				lastExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastExpressionIdx].GetOutputs(prgrm)[0])
				lastExpressionOutputTypeSig.Name = glbl.Name
				lastExpressionOutputTypeSig.Type = glbl.Type
				lastExpressionOutputTypeSig.Meta = glbl.Meta
				lastExpressionOutputTypeSig.Offset = glbl.Offset
			} else if lastExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastExpressionIdx].GetOutputs(prgrm)[0]); lastExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// An array or slice global, pkg.A[i], is found by name like a global of this package.
				lastExpressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastExpressionOutputTypeSig.Meta))
				lastExpressionOutputArg.Name = glbl.Name
				lastExpressionOutputTypeSig.Name = glbl.Name
				lastExpressionOutputArg.Package = glbl.Package
				lastExpressionOutputTypeSig.Package = glbl.Package
			} else {
				panic("type is not known")
			}
		} else if _, found := imp.Functions[identGid]; found {
			// A function of imp, not of its imports (skycoin/cx#9). Like a local
			// function name it is a call only with parentheses (skycoin-cx#48).
			leftExprOutputTypeSig.Name = identGid
			if leftExprOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				prgrm.GetCXArg(ast.CXArgumentIndex(leftExprOutputTypeSig.Meta)).Name = identGid
			}
		} else if strct, err := prgrm.GetStruct(identGid, imp.Name); err == nil {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1196.
			lastExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastExpressionIdx].GetOutputs(prgrm)[0])
			var lastExpressionOutputArg *ast.CXArgument
			if lastExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1198.
				lastExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastExpressionOutputTypeSig.Meta))
			} else {
				panic("type is not cx argument deprecate\n\n")
			}

			lastExpressionOutputArg.StructType = strct
		} else {
			file, line, column := CurrentFile, LineNo, prgrm.CXProgramCompileErrorColumn(CurrentFile, LineNo)
			if details := leftExprOutputTypeSig.ArgDetails; details != ast.CXArgumentDebugGidNil {
				file, line, column = ast.CXArgumentDebugPosition(prgrm, details)
			}
			name := prgrm.CXProgramNameText(imp.Name) + "." + ident
			// A package whose import failed has no members, and its import is the error.
			if !prgrm.CXProgramImportFailed(imp.Name) && !renamedOpcodeRecord(prgrm, file, line, name) {
				prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodePackageIdentifierMissing, file, line, column, ast.CompileErrorOperands{int64(imp.Name), prgrm.CXProgramCompileErrorText(ident)})
			}
			return postfixFieldInvalidate(prgrm, prevExprs, lastExpressionIdx, name)
		}
	} else {
		// then left is not a package name
		if cxpackages.IsDefaultPackage(prgrm.CXProgramNameText(leftExprOutputTypeSig.Name)) {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeBuiltinPackageNotImported, leftExprOutputTypeSig.ArgDetails, ast.CompileErrorOperands{int64(leftExprOutputTypeSig.Name), int64(leftExprOutputTypeSig.Name)})
			return postfixFieldInvalidate(prgrm, prevExprs, lastExpressionIdx, prgrm.CXProgramNameText(leftExprOutputTypeSig.Name)+"."+ident)
		}

		// then it's a struct
		_, leftExprField := ast.MakeArgument(prgrm, identGid, CurrentFile, LineNo)
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S1200.
		leftPkg, err := prgrm.GetPackageFromArray(leftExprOutputTypeSig.Package)
		if err != nil {
			panic(err)
		}
		leftExprField.SetType(types.IDENTIFIER)
		leftExprField.SetPackage(leftPkg)
		leftExprFieldIdx := ast.CXArgumentIndex(leftExprField.Index)

		var leftExprIdx int
		if leftExprOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			leftExprIdx = leftExprOutputTypeSig.Meta

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1761.
			prgrm.GetCXArg(ast.CXArgumentIndex(leftExprIdx)).Fields = append(prgrm.GetCXArg(ast.CXArgumentIndex(leftExprIdx)).Fields, leftExprFieldIdx)
		} else if leftExprOutputTypeSig.Type == ast.TYPE_ATOMIC || leftExprOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// TODO: implement correct way when it is possible
			// temporarily return back to cx arg deprecate
			// since the arg will become a struct
			_, arg := ast.MakeArgument(prgrm, leftExprOutputTypeSig.Name, CurrentFile, LineNo) // fix: line numbers in errors sometimes report +1 or -1. Issue #195
			arg.SetType(types.IDENTIFIER)
			arg.Name = leftExprOutputTypeSig.Name
			arg.Package = leftExprOutputTypeSig.Package
			arg.PassBy = leftExprOutputTypeSig.PassBy

			argIdx := ast.CXArgumentIndex(arg.Index)
			leftExprOutputTypeSig.Type = ast.TYPE_CXARGUMENT_DEPRECATE
			leftExprOutputTypeSig.Meta = int(argIdx)

			leftExprIdx = int(argIdx)
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1762.
			prgrm.GetCXArg(ast.CXArgumentIndex(leftExprIdx)).Fields = append(prgrm.GetCXArg(ast.CXArgumentIndex(leftExprIdx)).Fields, leftExprFieldIdx)
		} else if leftExprOutputTypeSig.Type == ast.TYPE_STRUCT {
			// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
			// Now an arena getter, site S1204.
			structDetails := prgrm.GetCXTypeSignatureStructFromArray(leftExprOutputTypeSig.Meta)
			structDetails.Fields = append(structDetails.Fields, leftExprFieldIdx)
		} else {
			panic("type is not known")
		}
	}

	return prevExprs
}

// renamedOpcodeRecord records the compile error for name at file and line
// when it is a former opcode name, see constants.OpcodeRenameTable.
func renamedOpcodeRecord(prgrm *ast.CXProgram, file ast.SourceFileGid, line int, name string) bool {
	rename, found := constants.OpcodeRenamed(name)
	if !found {
		return false
	}
	operands := ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(name), prgrm.CXProgramCompileErrorText(rename.NewName), rename.Version}
	oldPkg, _, _ := strings.Cut(name, ".")
	if pkg, _, cut := strings.Cut(rename.NewName, "."); cut && pkg != oldPkg && cxpackages.IsDefaultPackage(pkg) {
		operands[3] = prgrm.CXProgramCompileErrorText(pkg)
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeOpcodeRenamedImport, file, line, operands)
		return true
	}
	prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeOpcodeRenamed, file, line, operands)
	return true
}

// MethodCallValue gives the method call that ends exprs a result variable,
// which ProcessMethodCall types, and appends an expression that reads it.
func MethodCallValue(prgrm *ast.CXProgram, exprs []ast.CXExpression) []ast.CXExpression {
	if len(exprs) == 0 || !exprs[len(exprs)-1].IsMethodCall() {
		return exprs
	}
	callOp := &prgrm.CXAtomicOps[exprs[len(exprs)-1].Index]
	if len(callOp.GetOutputs(prgrm)) > 0 {
		return exprs
	}
	return append(exprs, primaryIdentifierExpressions(prgrm, methodCallResultAdd(prgrm, callOp))...)
}

// methodCallResultAdd adds a result variable to the outputs of a method call
// and returns its name.
func methodCallResultAdd(prgrm *ast.CXProgram, callOp *ast.CXAtomicOperator) stringpool.StringGid {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	resultName := generateTempVarName(prgrm, constants.METHOD_RESULT_PREFIX)
	// A local name, so the result is declared even when typed by a signature.
	fn, err := prgrm.GetCurrentFunction()
	if err != nil {
		panic(err)
	}
	if err := fn.AddLocalVariableName(resultName); err != nil {
		panic(err)
	}
	_, result := ast.MakeArgument(prgrm, resultName, CurrentFile, LineNo)
	result.Type = types.UNDEFINED
	result.Package = ast.CXPackageIndex(pkg.Index)
	result.PreviouslyDeclared = true
	callOp.AddOutput(prgrm, ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, result))
	return resultName
}

// MethodCallStatement undoes MethodCallValue for a method call whose result
// is not used, so exprs end with the method call again.
func MethodCallStatement(prgrm *ast.CXProgram, exprs []ast.CXExpression) []ast.CXExpression {
	if len(exprs) < 2 || !exprs[len(exprs)-2].IsMethodCall() || exprs[len(exprs)-1].Type == ast.CX_LINE {
		return exprs
	}
	readOp := &prgrm.CXAtomicOps[exprs[len(exprs)-1].Index]
	callOp := &prgrm.CXAtomicOps[exprs[len(exprs)-2].Index]
	if prgrm.GetFunctionFromArray(readOp.Operator) != nil || len(callOp.GetOutputs(prgrm)) != 1 || len(readOp.GetOutputs(prgrm)) != 1 {
		return exprs
	}
	resultTypeSig := prgrm.GetCXTypeSignatureFromArray(callOp.GetOutputs(prgrm)[0])
	readTypeSig := prgrm.GetCXTypeSignatureFromArray(readOp.GetOutputs(prgrm)[0])
	if readTypeSig.Name != resultTypeSig.Name || readTypeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return exprs
	}
	read := prgrm.GetCXArg(ast.CXArgumentIndex(readTypeSig.Meta))
	if len(read.Fields) > 0 || len(read.Indexes) > 0 || len(read.DereferenceOperations) > 0 {
		return exprs
	}
	callOp.Outputs = nil
	if fn, err := prgrm.GetCurrentFunction(); err == nil {
		if err := fn.RemoveLocalVariableFromArray(resultTypeSig.Name); err != nil {
			panic(err)
		}
	}
	return exprs[:len(exprs)-1]
}

// functionCallRepeated records the compile error for a call of a call
// result, as in f() () (skycoin-cx#22).
func functionCallRepeated(prgrm *ast.CXProgram, prevExprs []ast.CXExpression) bool {
	if len(prevExprs) == 0 {
		return false
	}
	last := prevExprs[len(prevExprs)-1]
	if last.ExpressionType != ast.CXEXPR_FUNCTION_CALL && !last.IsMethodCall() {
		return false
	}
	prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeCallResultCall, CurrentFile, LineNo, ast.CompileErrorOperands{})
	return true
}

// functionDeclared reports whether the package of sig declares a function
// named sig.Name, which then shadows a built-in (skycoin-cx#294).
func functionDeclared(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) bool {
	if sig.Package < 0 {
		return false
	}
	pkg, err := prgrm.GetPackageFromArray(sig.Package)
	if err != nil {
		return false
	}
	_, found := pkg.Functions[sig.Name]
	return found
}

// ArgumentReadCopy returns a copy of a field, index or dereference read with
// its own arguments, so one expression can read and write it as in p.x++.
func ArgumentReadCopy(prgrm *ast.CXProgram, sigIdx ast.CXTypeSignatureIndex) ast.CXTypeSignatureIndex {
	sig := prgrm.GetCXTypeSignatureFromArray(sigIdx)
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return sigIdx
	}
	arg := prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta))
	if len(arg.Fields) == 0 && len(arg.Indexes) == 0 && len(arg.DereferenceOperations) == 0 {
		return sigIdx
	}
	return ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, argumentCopy(prgrm, arg))
}

// argumentCopy copies arg with its fields and indexes into new arguments.
func argumentCopy(prgrm *ast.CXProgram, arg *ast.CXArgument) *ast.CXArgument {
	details := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(arg.ArgDetails)
	_, argCopy := ast.MakeArgument(prgrm, arg.Name, details.FileName, details.FileLine)
	index, argDetails := argCopy.Index, argCopy.ArgDetails
	*argCopy = *arg
	argCopy.Index, argCopy.ArgDetails = index, argDetails
	argCopy.Lengths = append([]types.Pointer(nil), arg.Lengths...)
	argCopy.DereferenceOperations = append([]int(nil), arg.DereferenceOperations...)
	argCopy.DeclarationSpecifiers = append([]int(nil), arg.DeclarationSpecifiers...)
	argCopy.Indexes = make([]ast.CXTypeSignatureIndex, len(arg.Indexes))
	for i, indexSigIdx := range arg.Indexes {
		indexSig := prgrm.GetCXTypeSignatureFromArray(indexSigIdx)
		argCopy.Indexes[i] = indexSigIdx
		if indexSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			argCopy.Indexes[i] = ast.AllocCXTypeSignatureOfCXArgDeprecate(prgrm, argumentCopy(prgrm, prgrm.GetCXArg(ast.CXArgumentIndex(indexSig.Meta))))
		}
	}
	argCopy.Fields = make([]ast.CXArgumentIndex, len(arg.Fields))
	for i, fieldIdx := range arg.Fields {
		argCopy.Fields[i] = ast.CXArgumentIndex(argumentCopy(prgrm, prgrm.GetCXArg(fieldIdx)).Index)
	}
	return argCopy
}

// postfixCallResultArgument returns the argument of the result sig of a call
// that f().x reads, made for a struct or *T result that has none.
func postfixCallResultArgument(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) *ast.CXArgument {
	if sig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		return prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta))
	}
	strct := prgrm.GetCXTypeSignatureStructFromArray(sig.Meta).StructType
	arg := &ast.CXArgument{Type: types.STRUCT, StructType: strct, Size: strct.GetStructSize(prgrm), DeclarationSpecifiers: []int{constants.DECL_STRUCT}}
	if sig.Type == ast.TYPE_POINTER_STRUCT {
		arg.Type, arg.PointerTargetType, arg.Size = types.POINTER, types.STRUCT, types.POINTER_SIZE
		arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_POINTER)
	}
	return arg
}

// postfixFieldInvalidate makes the expression at lastExpressionIdx, a
// pkg.name that does not exist, a call of a stand in for name, see
// invalidFunctionIndex, so its uses report nothing more.
func postfixFieldInvalidate(prgrm *ast.CXProgram, prevExprs []ast.CXExpression, lastExpressionIdx int, name string) []ast.CXExpression {
	expression := &prgrm.CXAtomicOps[lastExpressionIdx]
	expression.Outputs = nil
	expression.Inputs = nil
	expression.Operator = invalidFunctionIndex(prgrm, expression.Package, prgrm.CXProgramNameIntern(name))
	return prevExprs
}
