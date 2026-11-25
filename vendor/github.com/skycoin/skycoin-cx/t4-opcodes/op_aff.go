package opcodes

import (
	"fmt"
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates query strs and
	// objects, which adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
	// "github.com/skycoin/skycoin/src/cipher/encoder"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

var onMessages = map[string]string{
	"arg-arg-input":    "Replace %s.Input.%d with %s",
	"arg-arg-output":   "Replace %s.Output.%d with %s",
	"arg-expr":         "Add ",
	"prgrm-arg-input":  "Print %s.Input.%d's value",
	"prgrm-arg-output": "Print %s.Output.%d's value",
}
var ofMessages = map[string]string{
	"arg-arg-input":    "Replace %[3]s with %[1]s.Input.%[2]d",
	"arg-arg-output":   "Replace %[3]s with %[1]s.Output.%[2]d",
	"strct-arg-input":  "Add %[1]s.Input.%[2]d as a new field of %s",
	"strct-arg-output": "Add %[1]s.Output.%[2]d as a new field of %s",
	"prgrm-arg-input":  "Print %[1]s.Input.%[2]d's value",
	"prgrm-arg-output": "Print %[1]s.Output.%[2]d's value",
}

// GetInferActions ...
// VIOLATION STRHOT-070: B, an affordance query runs compiler code by name, #412.
func GetInferActions(prgrm *ast.CXProgram, inp *ast.CXTypeSignature, fp types.Pointer) []string {
	inpOffset := ast.GetFinalOffset(prgrm, fp, nil, inp)

	off := types.Read_ptr(prgrm.Memory, inpOffset)

	l := types.Read_ptr(ast.GetSliceHeader(prgrm, ast.GetSliceOffset(prgrm, fp, inp)), types.POINTER_SIZE)

	result := make([]string, l)

	for c := types.Cast_int_to_ptr(0); c < l; c++ {
		elOff := types.Read_ptr(prgrm.Memory, off+types.OBJECT_HEADER_SIZE+constants.SLICE_HEADER_SIZE+c*types.POINTER_SIZE)
		result[c] = types.Read_str_data(prgrm.Memory, elOff)
	}

	return result
}

// VIOLATION STRHOT-071: B, an affordance query runs compiler code by name, #412.
// VIOLATION VMALLOC-019: an affordance query runs compiler code, #407.
func OpAffPrint(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inp1 := inputs[0]

	fmt.Println(GetInferActions(prgrm, inp1.TypeSignature, inp1.FramePointer))
	// for _, aff := range GetInferActions(inp1, fp) {
	// 	fmt.Println(aff)
	// }
}

// CallAffPredicate ...
func CallAffPredicate(prgrm *ast.CXProgram, fn *ast.CXFunction, predValue []byte) byte {
	prevCall := &prgrm.CallStack[prgrm.CallCounter]

	prgrm.CallCounter++
	newCall := &prgrm.CallStack[prgrm.CallCounter]
	newCall.Operator = fn
	newCallOperatorInputs := newCall.Operator.GetInputs(prgrm)
	newCallOperatorOutputs := newCall.Operator.GetOutputs(prgrm)
	newCall.Line = 0
	newCall.FramePointer = prgrm.Stack.Pointer

	prgrm.Stack.Pointer += newCall.Operator.Size

	newFP := newCall.FramePointer

	// wiping next mem frame (removing garbage)
	for c := types.Pointer(0); c < fn.Size; c++ {
		prgrm.Memory[newFP+c] = 0
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S1253.
	newCallOperatorInputTypeSig := prgrm.GetCXTypeSignatureFromArray(newCallOperatorInputs[0])

	// sending value to predicate function
	types.WriteSlice_byte(
		prgrm.Memory,
		ast.GetFinalOffset(prgrm, newFP, nil, newCallOperatorInputTypeSig),
		predValue)

	var inputs []ast.CXValue
	var outputs []ast.CXValue
	prevCC := prgrm.CallCounter
	for {
		call := &prgrm.CallStack[prgrm.CallCounter]
		err := call.Call(prgrm, &inputs, &outputs)
		if err != nil {
			panic(err)
		}
		if prgrm.CallCounter < prevCC {
			break
		}
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S1254.
	newCallOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(newCallOperatorOutputs[0])

	prevCall.Line--
	return types.GetSlice_byte(prgrm.Memory, ast.GetFinalOffset(prgrm,
		newCall.FramePointer,
		nil, newCallOperatorOutputTypeSig),
		newCallOperatorOutputTypeSig.GetSize(prgrm, false))[0]
}

// Used by QueryArgument to query inputs and then outputs from expressions.
// GCREACH: allocates query strs and objects, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-072: B, an affordance query runs compiler code by name, #412.
func queryParam(prgrm *ast.CXProgram, fn *ast.CXFunction, argsIdx []ast.CXArgumentIndex, exprLbl string, argOffsetB []byte, affOffset *types.Pointer) {
	for i, argIdx := range argsIdx {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1255.
		arg := prgrm.GetCXArgFromArray(argIdx)

		var typOffset types.Pointer
		elt := arg.GetAssignmentElement(prgrm)
		if elt.StructType != nil {
			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S1256.
			strctTypePkg, err := prgrm.GetPackageFromArray(elt.StructType.Package)
			if err != nil {
				panic(err)
			}

			// then it's struct type
			// typOffset = WriteObjectRetOff(encoder.Serialize(elt.StructType.Package.Name + "." + elt.StructType.Name))
			typOffset = ast.AllocateStr(prgrm, prgrm.CXProgramNameText(strctTypePkg.Name)+"."+prgrm.CXProgramNameText(elt.StructType.Name))
		} else {
			// then it's native type
			// typOffset = WriteObjectRetOff(encoder.Serialize(TypeNames[elt.Type]))
			typOffset = ast.AllocateStr(prgrm, elt.Type.Name())
		}

		// Name
		// argNameB := encoder.Serialize(arg.Name)
		// argNameOffset := int32(WriteObjectRetOff(argNameB))
		argNameOffset := ast.AllocateStr(prgrm, prgrm.CXProgramNameText(arg.Name))

		argOffset := ast.AllocateSeq(prgrm, types.OBJECT_HEADER_SIZE+types.STR_SIZE+types.I32_SIZE+types.STR_SIZE)
		types.Write_ptr(prgrm.Memory, argOffset+types.OBJECT_HEADER_SIZE, argNameOffset)

		// Index
		types.Write_ptr(prgrm.Memory, argOffset+types.OBJECT_HEADER_SIZE+types.STR_SIZE, types.Cast_int_to_ptr(i))

		// Type
		types.Write_ptr(prgrm.Memory, argOffset+types.OBJECT_HEADER_SIZE+types.STR_SIZE+types.I32_SIZE, typOffset)

		res := CallAffPredicate(prgrm, fn, prgrm.Memory[argOffset+types.OBJECT_HEADER_SIZE:argOffset+types.OBJECT_HEADER_SIZE+types.STR_SIZE+types.I32_SIZE+types.STR_SIZE])

		if res == 1 {
			*affOffset = ast.WriteToSlice(prgrm, *affOffset, argOffsetB)

			// affNameB := encoder.Serialize(fmt.Sprintf("%s.%d", exprLbl, i))
			// affNameOffset := AllocateSeq(len(affNameB))
			affNameOffset := ast.AllocateStr(prgrm, fmt.Sprintf("%s.%d", exprLbl, i))
			// WriteMemory(affNameOffset, affNameB)

			var affNameOffsetBytes [4]byte
			types.Write_ptr(affNameOffsetBytes[:], 0, affNameOffset)
			*affOffset = ast.WriteToSlice(prgrm, *affOffset, affNameOffsetBytes[:])
		}
	}
}

// QueryArgument ...
// VIOLATION STRHOT-073: B, an affordance query runs compiler code by name, #412.
func QueryArgument(prgrm *ast.CXProgram, fn *ast.CXFunction, expr *ast.CXExpression, argOffsetB []byte, affOffset *types.Pointer) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1257.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1258.
	cxAtomicOpFunction := prgrm.GetFunctionFromArray(cxAtomicOp.Function)

	for _, ex := range cxAtomicOpFunction.Expressions {
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S1259.
		exCXAtomicOp, err := prgrm.GetCXAtomicOp(ex.Index)
		if err != nil {
			panic(err)
		}

		if exCXAtomicOp.Label == stringpool.StringGidNil {
			// it's a non-labeled expression
			continue
		}

		var inputCXArgsIdxs []ast.CXArgumentIndex
		for _, inputIdx := range exCXAtomicOp.GetInputs(prgrm) {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1260.
			input := prgrm.GetCXTypeSignatureFromArray(inputIdx)
			if input.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				inputCXArgsIdxs = append(inputCXArgsIdxs, ast.CXArgumentIndex(input.Meta))
			} else {
				panic("type is not type cx argument deprecate\n\n")
			}
		}
		queryParam(prgrm, fn, inputCXArgsIdxs, prgrm.CXProgramNameText(exCXAtomicOp.Label)+".Input", argOffsetB, affOffset)

		var outputCXArgsIdxs []ast.CXArgumentIndex
		for _, outputIdx := range exCXAtomicOp.GetOutputs(prgrm) {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1261.
			output := prgrm.GetCXTypeSignatureFromArray(outputIdx)
			if output.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				outputCXArgsIdxs = append(outputCXArgsIdxs, ast.CXArgumentIndex(output.Meta))
			} else {
				panic("type is not type cx argument deprecate\n\n")
			}
		}
		queryParam(prgrm, fn, outputCXArgsIdxs, prgrm.CXProgramNameText(exCXAtomicOp.Label)+".Output", argOffsetB, affOffset)
	}
}

// QueryExpressions ...
// GCREACH: allocates query strs and objects, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-074: B, an affordance query runs compiler code by name, #412.
func QueryExpressions(prgrm *ast.CXProgram, fn *ast.CXFunction, expr *ast.CXExpression, exprOffsetB []byte, affOffset *types.Pointer) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1262.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1263.
	cxAtomicOpFunction := prgrm.GetFunctionFromArray(cxAtomicOp.Function)

	for _, ex := range cxAtomicOpFunction.Expressions {
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S1264.
		exCXAtomicOp, err := prgrm.GetCXAtomicOp(ex.Index)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S1265.
		exCXAtomicOpOperator := prgrm.GetFunctionFromArray(exCXAtomicOp.Operator)

		if exCXAtomicOpOperator == nil || exCXAtomicOp.Label == stringpool.StringGidNil {
			// then it's a variable declaration
			// or it's a non-labeled expression
			continue
		}

		// var opNameB []byte
		opNameOffset := types.Pointer(0)
		if exCXAtomicOpOperator.IsBuiltIn() {
			// opNameB = encoder.Serialize(OpNames[ex.Operator.OpCode])
			opNameOffset = ast.AllocateStr(prgrm, exCXAtomicOpOperator.BuiltInName())
		} else {
			// opNameB = encoder.Serialize(ex.Operator.Name)
			opNameOffset = ast.AllocateStr(prgrm, prgrm.CXProgramNameText(exCXAtomicOpOperator.Name))
		}

		// opNameOffset := AllocateSeq(len(opNameB))
		// WriteMemory(opNameOffset, opNameB)
		var opNameOffsetB [4]byte
		types.Write_ptr(opNameOffsetB[:], 0, opNameOffset)
		res := CallAffPredicate(prgrm, fn, opNameOffsetB[:])

		if res == 1 {
			*affOffset = ast.WriteToSlice(prgrm, *affOffset, exprOffsetB)

			// lblNameB := encoder.Serialize(ex.Label)
			// lblNameOffset := AllocateSeq(len(lblNameB))
			lblNameOffset := ast.AllocateStr(prgrm, prgrm.CXProgramNameText(exCXAtomicOp.Label))
			// WriteMemory(lblNameOffset, lblNameB)
			var lblNameOffsetB [4]byte
			types.Write_ptr(lblNameOffsetB[:], 0, lblNameOffset)
			*affOffset = ast.WriteToSlice(prgrm, *affOffset, lblNameOffsetB[:])
		}
	}
}

// TODO: remove params arg
// GCREACH: allocates query strs and objects, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-075: B, an affordance query runs compiler code by name, #412.
func getSignatureSlice(prgrm *ast.CXProgram, params []ast.CXArgumentIndex, paramsTypeSig []ast.CXTypeSignatureIndex) types.Pointer {
	var sliceOffset types.Pointer

	var arrCXArgs []*ast.CXArgument
	for _, paramIdx := range params {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1266.
		param := prgrm.GetCXArgFromArray(paramIdx)
		arrCXArgs = append(arrCXArgs, param)
	}
	for _, paramIdx := range paramsTypeSig {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1267.
		param := prgrm.GetCXTypeSignatureFromArray(paramIdx)
		if param.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1268.
			arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(param.Meta))
			arrCXArgs = append(arrCXArgs, arg)
		} else {
			panic("type is not type cx argument deprecate\n\n")
		}
	}

	for _, param := range arrCXArgs {
		var typOffset types.Pointer
		if param.StructType != nil {
			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S1269.
			strctTypePkg, err := prgrm.GetPackageFromArray(param.StructType.Package)
			if err != nil {
				panic(err)
			}

			// then it's struct type
			// typOffset = WriteObjectRetOff(encoder.Serialize(param.StructType.Package.Name + "." + param.StructType.Name))
			typOffset = ast.AllocateStr(prgrm, prgrm.CXProgramNameText(strctTypePkg.Name)+"."+prgrm.CXProgramNameText(param.StructType.Name))
		} else {
			// then it's native type
			// typOffset = WriteObjectRetOff(encoder.Serialize(TypeNames[param.Type]))
			typOffset = ast.AllocateStr(prgrm, param.Type.Name())
		}

		var typOffsetB [types.POINTER_SIZE]byte
		types.Write_ptr(typOffsetB[:], 0, typOffset)
		sliceOffset = ast.WriteToSlice(prgrm, sliceOffset, typOffsetB[:])
	}

	return sliceOffset
}

// Helper function for QueryStructure. Used to query all the structs in a particular package
// GCREACH: allocates query strs and objects, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-076: B, an affordance query runs compiler code by name, #412.
func queryStructsInPackage(prgrm *ast.CXProgram, fn *ast.CXFunction, strctOffsetB []byte, affOffset *types.Pointer, pkg *ast.CXPackage) {
	for _, fIdx := range pkg.Structs {
		f := prgrm.CXStructs[fIdx]
		// strctNameB := encoder.Serialize(f.Name)

		// strctNameOffset := WriteObjectRetOff(strctNameB)
		strctNameOffset := ast.AllocateStr(prgrm, prgrm.CXProgramNameText(f.Name))
		var strctNameOffsetB [types.POINTER_SIZE]byte
		types.Write_ptr(strctNameOffsetB[:], 0, strctNameOffset)

		strctOffset := ast.AllocateSeq(prgrm, types.OBJECT_HEADER_SIZE+types.STR_SIZE)
		// Name
		types.WriteSlice_byte(prgrm.Memory, strctOffset+types.OBJECT_HEADER_SIZE, strctNameOffsetB[:])

		val := prgrm.Memory[strctOffset+types.OBJECT_HEADER_SIZE : strctOffset+types.OBJECT_HEADER_SIZE+types.STR_SIZE]
		res := CallAffPredicate(prgrm, fn, val)

		if res == 1 {
			*affOffset = ast.WriteToSlice(prgrm, *affOffset, strctOffsetB)
			*affOffset = ast.WriteToSlice(prgrm, *affOffset, strctNameOffsetB[:])
		}
	}
}

// QueryStructure ...
func QueryStructure(prgrm *ast.CXProgram, fn *ast.CXFunction, expr *ast.CXExpression, strctOffsetB []byte, affOffset *types.Pointer) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1270.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S1271.
	pkg, err := prgrm.GetPackageFromArray(cxAtomicOp.Package)
	if err != nil {
		panic(err)
	}

	queryStructsInPackage(prgrm, fn, strctOffsetB, affOffset, pkg)
	for _, impIdx := range pkg.Imports {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S1272.
		imp, err := prgrm.GetPackageFromArray(impIdx)
		if err != nil {
			panic(err)
		}
		queryStructsInPackage(prgrm, fn, strctOffsetB, affOffset, imp)
	}
}

// QueryFunction ...
// GCREACH: allocates query strs and objects, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-077: B, an affordance query runs compiler code by name, #412.
func QueryFunction(prgrm *ast.CXProgram, fn *ast.CXFunction, expr *ast.CXExpression, fnOffsetB []byte, affOffset *types.Pointer) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1273.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S1274.
	pkg, err := prgrm.GetPackageFromArray(cxAtomicOp.Package)
	if err != nil {
		panic(err)
	}
	for _, fIdx := range pkg.Functions {
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S1275.
		f := prgrm.GetFunctionFromArray(fIdx)

		if f.Name == prgrm.CxIrState.CxIrStateNameSysInit {
			continue
		}

		// var opNameB []byte
		opNameOffset := types.Pointer(0)
		if f.IsBuiltIn() {
			// opNameB = encoder.Serialize(OpNames[f.OpCode])
			opNameOffset = ast.AllocateStr(prgrm, f.BuiltInName())
		} else {
			// opNameB = encoder.Serialize(f.Name)
			opNameOffset = ast.AllocateStr(prgrm, prgrm.CXProgramNameText(f.Name))
		}

		var opNameOffsetB [types.POINTER_SIZE]byte
		// WriteMemI32(opNameOffsetB[:], 0, int32(WriteObjectRetOff(opNameB)))
		types.Write_ptr(opNameOffsetB[:], 0, opNameOffset)

		inpSigOffset := getSignatureSlice(prgrm, nil, f.GetInputs(prgrm))
		outSigOffset := getSignatureSlice(prgrm, nil, f.GetOutputs(prgrm))

		fnOffset := ast.AllocateSeq(prgrm, types.OBJECT_HEADER_SIZE+types.STR_SIZE+types.POINTER_SIZE+types.POINTER_SIZE)
		// Name
		types.WriteSlice_byte(prgrm.Memory, fnOffset+types.OBJECT_HEADER_SIZE, opNameOffsetB[:])
		// InputSignature
		types.Write_ptr(prgrm.Memory, fnOffset+types.OBJECT_HEADER_SIZE+types.POINTER_SIZE, inpSigOffset)
		// OutputSignature
		types.Write_ptr(prgrm.Memory, fnOffset+types.OBJECT_HEADER_SIZE+types.POINTER_SIZE+types.POINTER_SIZE, outSigOffset)

		val := prgrm.Memory[fnOffset+types.OBJECT_HEADER_SIZE : fnOffset+types.OBJECT_HEADER_SIZE+types.STR_SIZE+types.POINTER_SIZE+types.POINTER_SIZE]
		res := CallAffPredicate(prgrm, fn, val)

		if res == 1 {
			*affOffset = ast.WriteToSlice(prgrm, *affOffset, fnOffsetB)
			*affOffset = ast.WriteToSlice(prgrm, *affOffset, opNameOffsetB[:])
		}
	}
}

// QueryCaller ...
// GCREACH: allocates query strs and objects, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-078: B, an affordance query runs compiler code by name, #412.
func QueryCaller(prgrm *ast.CXProgram, fn *ast.CXFunction, expr *ast.CXExpression, callerOffsetB []byte, affOffset *types.Pointer) {
	if prgrm.CallCounter == 0 {
		// then it's entry point
		return
	}

	call := prgrm.CallStack[prgrm.CallCounter-1]

	// var opNameB []byte
	opNameOffset := types.Pointer(0)
	if call.Operator.IsBuiltIn() {
		// opNameB = encoder.Serialize(OpNames[call.Operator.OpCode])
		opNameOffset = ast.AllocateStr(prgrm, call.Operator.BuiltInName())
	} else {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S1276.
		opPkg, err := prgrm.GetPackageFromArray(call.Operator.Package)
		if err != nil {
			panic(err)
		}
		// opNameB = encoder.Serialize(call.Operator.Package.Name + "." + call.Operator.Name)
		opNameOffset = ast.AllocateStr(prgrm, prgrm.CXProgramNameText(opPkg.Name)+"."+prgrm.CXProgramNameText(call.Operator.Name))
	}

	callOffset := ast.AllocateSeq(prgrm, types.OBJECT_HEADER_SIZE+types.STR_SIZE+types.I32_SIZE)

	// FnName
	var opNameOffsetB [4]byte
	// WriteMemI32(opNameOffsetB[:], 0, int32(WriteObjectRetOff(opNameB)))
	types.Write_ptr(opNameOffsetB[:], 0, opNameOffset)
	types.WriteSlice_byte(prgrm.Memory, callOffset+types.OBJECT_HEADER_SIZE, opNameOffsetB[:])

	// FnSize
	types.Write_ptr(prgrm.Memory, callOffset+types.OBJECT_HEADER_SIZE+types.STR_SIZE, call.Operator.Size)

	res := CallAffPredicate(prgrm, fn, prgrm.Memory[callOffset+types.OBJECT_HEADER_SIZE:callOffset+types.OBJECT_HEADER_SIZE+types.STR_SIZE+types.I32_SIZE])

	if res == 1 {
		*affOffset = ast.WriteToSlice(prgrm, *affOffset, callerOffsetB)
	}
}

// QueryProgram ...
// GCREACH: allocates query strs and objects, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-079: B, an affordance query runs compiler code by name, #412.
func QueryProgram(prgrm *ast.CXProgram, fn *ast.CXFunction, expr *ast.CXExpression, prgrmOffsetB []byte, affOffset *types.Pointer) {
	prgrmOffset := ast.AllocateSeq(prgrm, types.OBJECT_HEADER_SIZE+types.I32_SIZE+types.I64_SIZE+types.STR_SIZE+types.I32_SIZE)
	// Callcounter
	types.Write_ptr(prgrm.Memory, prgrmOffset+types.OBJECT_HEADER_SIZE, prgrm.CallCounter)
	// HeapUsed
	types.Write_ptr(prgrm.Memory, prgrmOffset+types.OBJECT_HEADER_SIZE+types.I32_SIZE, prgrm.Heap.Pointer)

	// Caller
	if prgrm.CallCounter != 0 {
		// then it's not just entry point
		call := prgrm.CallStack[prgrm.CallCounter-1]

		// var opNameB []byte
		opNameOffset := types.Pointer(0)
		if call.Operator.IsBuiltIn() {
			// opNameB = encoder.Serialize(OpNames[call.Operator.OpCode])
			opNameOffset = ast.AllocateStr(prgrm, call.Operator.BuiltInName())
		} else {
			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S1277.
			opPkg, err := prgrm.GetPackageFromArray(call.Operator.Package)
			if err != nil {
				panic(err)
			}

			// opNameB = encoder.Serialize(call.Operator.Package.Name + "." + call.Operator.Name)
			opNameOffset = ast.AllocateStr(prgrm, prgrm.CXProgramNameText(opPkg.Name)+"."+prgrm.CXProgramNameText(call.Operator.Name))
		}

		// callOffset := AllocateSeq(OBJECT_HEADER_SIZE + STR_SIZE + I32_SIZE)
		// FnName
		var opNameOffsetB [4]byte
		// WriteMemI32(opNameOffsetB[:], 0, int32(WriteObjectRetOff(opNameB)))
		types.Write_ptr(opNameOffsetB[:], 0, opNameOffset)
		types.WriteSlice_byte(prgrm.Memory, prgrmOffset+types.OBJECT_HEADER_SIZE+types.I32_SIZE+types.I64_SIZE, opNameOffsetB[:])
		// FnSize
		types.Write_ptr(prgrm.Memory, prgrmOffset+types.OBJECT_HEADER_SIZE+types.I32_SIZE+types.I64_SIZE+types.STR_SIZE, call.Operator.Size)
	}

	res := CallAffPredicate(prgrm, fn, prgrm.Memory[prgrmOffset+types.OBJECT_HEADER_SIZE:prgrmOffset+types.OBJECT_HEADER_SIZE+types.I32_SIZE+types.I64_SIZE+types.STR_SIZE+types.I32_SIZE])

	if res == 1 {
		*affOffset = ast.WriteToSlice(prgrm, *affOffset, prgrmOffsetB)
		*affOffset = ast.WriteToSlice(prgrm, *affOffset, prgrmOffsetB)
	}
}

func getTarget(prgrm *ast.CXProgram, inp2 *ast.CXTypeSignature, fp types.Pointer, tgtElt *string, tgtArgType *string, tgtArgIndex *int,
	tgtPkg *ast.CXPackage, tgtFn *ast.CXFunction, tgtExpr *ast.CXExpression) {
	for _, aff := range GetInferActions(prgrm, inp2, fp) {
		switch aff {
		case "prgrm":
			*tgtElt = "prgrm"
		case "Pkg":
			*tgtElt = "Pkg"
		case "strct":
			*tgtElt = "strct"
		case "fn":
			*tgtElt = "fn"
		case "expr":
			*tgtElt = "expr"
		case "rec":
			*tgtElt = "rec"
		case "inp":
			*tgtElt = "inp"
		case "out":
			*tgtElt = "out"
		default:
			switch *tgtElt {
			case "Pkg":
				if pkg, err := affPackage(prgrm, aff); err == nil {
					// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
					// Now an object copy, site S1278.
					*tgtPkg = *pkg
				} else {
					panic(err)
				}
			case "fn":
				if fn, err := affFunction(prgrm, tgtPkg, aff); err == nil {
					// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
					// Now an object copy, site S1279.
					*tgtFn = *fn
				} else {
					panic(err)
				}
			case "expr":
				if expr, err := tgtFn.GetExpressionByLabel(prgrm, aff); err == nil {
					// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
					// Now an object copy, site S1280.
					*tgtExpr = *expr
				} else {
					panic(err)
				}
			case "inp":
				*tgtArgType = "inp"
				*tgtElt = "arg"
				i, err := strconv.ParseInt(aff, 10, 32)

				*tgtArgIndex = int(i)

				if err != nil {
					panic(err)
				}
			case "out":
				*tgtArgType = "out"
				*tgtElt = "arg"
				i, err := strconv.ParseInt(aff, 10, 32)

				*tgtArgIndex = int(i)

				if err != nil {
					panic(err)
				}
			}
		}
	}
}

func getAffordances(prgrm *ast.CXProgram, inp1 *ast.CXTypeSignature, fp types.Pointer,
	tgtElt string, tgtArgType string, tgtArgIndex int,
	tgtPkg *ast.CXPackage, tgtFn *ast.CXFunction, tgtExpr *ast.CXExpression,
	affMsgs map[string]string,
	affs *[]string) {
	var fltrElt string
	elts := GetInferActions(prgrm, inp1, fp)

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1281.
	tgtExprAtomicOp, err := prgrm.GetCXAtomicOp(tgtExpr.Index)
	if err != nil {
		panic(err)
	}

	// for _, elt := range elts {
	for c := 0; c < len(elts); c++ {
		elt := elts[c]
		switch elt {
		case "arg":
			fltrElt = "arg"
		case "expr":
			fltrElt = "expr"
		case "fn":
			fltrElt = "fn"
		case "strct":
			fltrElt = "strct"
		case "Pkg":
			fltrElt = "Pkg"
		case "prgrm":
			fltrElt = "prgrm"
			// skipping the extra "prgrm"
			c++
			switch tgtElt {
			case "arg":
				if tgtArgType == "inp" {
					if msg, ok := affMsgs["prgrm-arg-input"]; ok {
						*affs = append(*affs, fmt.Sprintf(msg, prgrm.CXProgramNameText(tgtExprAtomicOp.Label), tgtArgIndex))
					}
				} else {
					if msg, ok := affMsgs["prgrm-arg-output"]; ok {
						*affs = append(*affs, fmt.Sprintf(msg, prgrm.CXProgramNameText(tgtExprAtomicOp.Label), tgtArgIndex))
					}
				}
			case "prgrm":
				*affs = append(*affs, "Run program")
			}
		default:
			switch fltrElt {
			case "arg":
				switch tgtElt {
				case "arg":
					if tgtArgType == "inp" {
						if msg, ok := affMsgs["arg-arg-input"]; ok {
							*affs = append(*affs, fmt.Sprintf(msg, prgrm.CXProgramNameText(tgtExprAtomicOp.Label), tgtArgIndex, elt))
						}
					} else {
						if msg, ok := affMsgs["arg-arg-output"]; ok {
							*affs = append(*affs, fmt.Sprintf(msg, prgrm.CXProgramNameText(tgtExprAtomicOp.Label), tgtArgIndex, elt))
						}
					}
				case "prgrm":
					*affs = append(*affs, "Print FA's value")
				}
			case "expr":
				if expr, err := tgtFn.GetExpressionByLabel(prgrm, elt); err == nil {
					_ = expr
					switch tgtElt {
					case "arg":
						*affs = append(*affs, "Replace TA by FE")
					case "fn":
						*affs = append(*affs, "Add FE to TF")
					case "prgrm":
						*affs = append(*affs, "Call FE")
					}
				} else {
					panic(err)
				}
			case "fn":
				if fn, err := affFunction(prgrm, tgtPkg, elt); err == nil {
					_ = fn
					switch tgtElt {
					case "arg":
						*affs = append(*affs, "Replace TA by a call to FF")
					case "expr":
						*affs = append(*affs, "Change TE's operator to FF")
					case "Pkg":
						// affs = append(affs, fmt.Sprintf("[%s.Operator = %s]", tgtExpr.Label, fn.Name))
						*affs = append(*affs, "Move FF to TP")
					case "prgrm":
						*affs = append(*affs, "Call FF")
					}
				} else {
					panic(err)
				}
			case "strct":
				switch tgtElt {
				case "arg":
					if msg, ok := affMsgs["arg-strct"]; ok {
						*affs = append(*affs, fmt.Sprintf(msg, prgrm.CXProgramNameText(tgtExprAtomicOp.Label), tgtArgIndex, elt))
					}
					if tgtArgType == "inp" {
						if msg, ok := affMsgs["strct-arg-input"]; ok {
							*affs = append(*affs, fmt.Sprintf(msg, prgrm.CXProgramNameText(tgtExprAtomicOp.Label), tgtArgIndex, elt))
						}
					} else {
						if msg, ok := affMsgs["strct-arg-output"]; ok {
							*affs = append(*affs, fmt.Sprintf(msg, prgrm.CXProgramNameText(tgtExprAtomicOp.Label), tgtArgIndex, elt))
						}
					}
				case "fn":
					*affs = append(*affs, "Add or change TF's receiver to FS")
				case "Pkg":
					*affs = append(*affs, "Move FS to TP")
				}
			case "Pkg":
				if pkg, err := affPackage(prgrm, elt); err == nil {
					_ = pkg
					switch tgtElt {
					case "Pkg":
						*affs = append(*affs, "Make TP import FP")
					}
				} else {
					panic(err)
				}
				// case "prgrm":
				// 	switch tgtElt {
				// 	case "prgrm":
				// 		affs = append(affs, "Run program")
				// 	}
			}
		}
	}
}

// func opAffOn(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
// 	inp1, inp2 := inputs[0].Arg, inputs[1].Arg

// 	prevPkgIdx := prgrm.CurrentPackage
// 	prevPkg, err := prgrm.GetPackageFromArray(prevPkgIdx)
// 	if err != nil {
// 		panic(err)
// 	}

// 	prevFnIdx := prevPkg.CurrentFunction
// 	prevFn, err := prgrm.GetFunctionFromArray(prevFnIdx)
// 	if err != nil {
// 		panic(err)
// 	}
// 	prevExpr := prevFn.CurrentExpression

// 	call := prgrm.GetCurrentCall()

// 	expr := call.Operator.Expressions[call.Line]
// 	fp := inputs[0].FramePointer

// 	var tgtPkg = ast.CXPackage(*prevPkg)

// 	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
// 	if err != nil {
// 		panic(err)
// 	}
// 	cxAtomicOpFunction, err := prgrm.GetFunctionFromArray(cxAtomicOp.Function)
// 	if err != nil {
// 		panic(err)
// 	}

// 	var tgtFn = ast.CXFunction(*cxAtomicOpFunction)
// 	var tgtExpr = ast.CXExpression(*prevExpr)

// 	// processing the target
// 	var tgtElt string
// 	var tgtArgType string
// 	var tgtArgIndex int

// 	getTarget(prgrm, inp2, fp, &tgtElt, &tgtArgType, &tgtArgIndex, &tgtPkg, &tgtFn, &tgtExpr)

// 	// var affPkg *CXPackage = prevPkg
// 	// var affFn *CXFunction = prevFn
// 	// var affExpr *CXExpression = prevExpr

// 	// processing the affordances
// 	var affs []string
// 	getAffordances(prgrm, inp1, fp, tgtElt, tgtArgType, tgtArgIndex, &tgtPkg, &tgtFn, &tgtExpr, onMessages, &affs)

// 	// returning to previous state
// 	prgrm.CurrentPackage = prevPkgIdx
// 	prevPkg.CurrentFunction = prevFnIdx
// 	prevFn.CurrentExpression = prevExpr

// 	for i, aff := range affs {
// 		fmt.Printf("%d - %s\n", i, aff)
// 	}
// }

// func opAffOf(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
// 	inp1, inp2 := inputs[0].Arg, inputs[1].Arg

// 	prevPkgIdx := prgrm.CurrentPackage
// 	prevPkg, err := prgrm.GetPackageFromArray(prevPkgIdx)
// 	if err != nil {
// 		panic(err)
// 	}

// 	prevFnIdx := prevPkg.CurrentFunction
// 	prevFn, err := prgrm.GetFunctionFromArray(prevFnIdx)
// 	if err != nil {
// 		panic(err)
// 	}
// 	prevExpr := prevFn.CurrentExpression

// 	call := prgrm.GetCurrentCall()
// 	expr := call.Operator.Expressions[call.Line]
// 	fp := inputs[0].FramePointer

// 	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
// 	if err != nil {
// 		panic(err)
// 	}

// 	cxAtomicOpFunction, err := prgrm.GetFunctionFromArray(cxAtomicOp.Function)
// 	if err != nil {
// 		panic(err)
// 	}

// 	opPkg, err := prgrm.GetPackageFromArray(cxAtomicOp.Package)
// 	if err != nil {
// 		panic(err)
// 	}
// 	var tgtPkg = ast.CXPackage(*opPkg)
// 	var tgtFn = ast.CXFunction(*cxAtomicOpFunction)
// 	var tgtExpr = ast.CXExpression(*prevExpr)

// 	// processing the target
// 	var tgtElt string
// 	var tgtArgType string
// 	var tgtArgIndex int

// 	getTarget(prgrm, inp2, fp, &tgtElt, &tgtArgType, &tgtArgIndex, &tgtPkg, &tgtFn, &tgtExpr)

// 	// processing the affordances
// 	var affs []string
// 	getAffordances(prgrm, inp1, fp, tgtElt, tgtArgType, tgtArgIndex, &tgtPkg, &tgtFn, &tgtExpr, ofMessages, &affs)

// 	// returning to previous state
// 	prgrm.CurrentPackage = prevPkgIdx
// 	prevPkg.CurrentFunction = prevFnIdx
// 	prevFn.CurrentExpression = prevExpr

// 	for i, aff := range affs {
// 		fmt.Printf("%d - %s\n", i, aff)
// 	}
// }

func readStrctAff(prgrm *ast.CXProgram, aff string, tgtPkg *ast.CXPackage) *ast.CXStruct {
	gid, found := prgrm.CXProgramNameFind(aff)
	if !found {
		panic(fmt.Errorf("struct '%s' not found in package '%s'", aff, prgrm.CXProgramNameText(tgtPkg.Name)))
	}
	strct, err := tgtPkg.GetStruct(prgrm, gid)
	if err != nil {
		panic(err)
	}

	return strct
}

func readArgAff(prgrm *ast.CXProgram, aff string, tgtFn *ast.CXFunction) *ast.CXArgument {
	var affExpr *ast.CXExpression
	var lIdx int
	var rIdx int
	var ch rune

	for _, ch = range aff {
		if ch == '.' {
			exprLbl := aff[lIdx:rIdx]
			for _, expr := range tgtFn.Expressions {
				if expr.Type == ast.CX_LINE {
					continue
				}
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S1282.
				cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}

				if exprLbl == prgrm.CXProgramNameText(cxAtomicOp.Label) {
					affExpr = &expr
					rIdx++
					break
				}
			}
		}

		if affExpr != nil {
			break
		}

		rIdx++
	}

	lIdx = rIdx

	var argType string

	for _, ch = range aff[lIdx:] {
		if ch == '.' {
			argType = aff[lIdx:rIdx]
			rIdx++
			break
		}

		if argType != "" {
			break
		}

		rIdx++
	}

	lIdx = rIdx

	argIdx, err := strconv.ParseInt(aff[lIdx:], 10, 32)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1283.
	affExprAtomicOp, err := prgrm.GetCXAtomicOp(affExpr.Index)
	if err != nil {
		panic(err)
	}

	if argType == "Input" {

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1284.
		affExprAtomicOpInputTypeSig := prgrm.GetCXTypeSignatureFromArray(affExprAtomicOp.GetInputs(prgrm)[argIdx])
		var arg *ast.CXArgument
		if affExprAtomicOpInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1286.
			arg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(affExprAtomicOpInputTypeSig.Meta))
		} else {
			panic("type is not a cx argumetn deprecate\n\n")
		}

		return arg
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S1287.
	affExprAtomicOpOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(affExprAtomicOp.GetOutputs(prgrm)[argIdx])
	var arg *ast.CXArgument
	if affExprAtomicOpOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1289.
		arg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(affExprAtomicOpOutputTypeSig.Meta))
	} else {
		panic("type is not a cx argumetn deprecate\n\n")
	}

	return arg
}

// func opAffInform(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
// 	inp1, inp2, inp3 := inputs[0].Arg, inputs[1].Arg, inputs[2].Arg

// 	call := prgrm.GetCurrentCall()
// 	expr := call.Operator.Expressions[call.Line]
// 	fp := inputs[0].FramePointer

// 	prevPkgIdx := prgrm.CurrentPackage
// 	prevPkg, err := prgrm.GetPackageFromArray(prevPkgIdx)
// 	if err != nil {
// 		panic(err)
// 	}

// 	prevFnIdx := prevPkg.CurrentFunction
// 	prevFn, err := prgrm.GetFunctionFromArray(prevFnIdx)
// 	if err != nil {
// 		panic(err)
// 	}
// 	prevExpr := prevFn.CurrentExpression

// 	var tgtPkg = ast.CXPackage(*prevPkg)

// 	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
// 	if err != nil {
// 		panic(err)
// 	}

// 	cxAtomicOpFunction, err := prgrm.GetFunctionFromArray(cxAtomicOp.Function)
// 	if err != nil {
// 		panic(err)
// 	}

// 	var tgtFn = ast.CXFunction(*cxAtomicOpFunction)
// 	var tgtExpr = ast.CXExpression(*prevExpr)

// 	// processing the target
// 	var tgtElt string
// 	var tgtArgType string
// 	var tgtArgIndex int

// 	getTarget(prgrm, inp3, fp, &tgtElt, &tgtArgType, &tgtArgIndex, &tgtPkg, &tgtFn, &tgtExpr)

// 	tgtExprAtomicOp, _, err := prgrm.GetOperation(&tgtExpr)
// 	if err != nil {
// 		panic(err)
// 	}

// 	elts := GetInferActions(prgrm, inp1, fp)
// 	eltIdx := types.Read_ptr(prgrm.Memory, ast.GetFinalOffset(prgrm, fp, inp2))
// 	eltType := elts[eltIdx*2]
// 	elt := elts[eltIdx*2+1]

// 	switch eltType {
// 	case "arg":
// 		switch tgtElt {
// 		case "arg":
// 			if tgtArgType == "inp" {
// 				tgtExprAtomicOp.Inputs[tgtArgIndex] = readArgAff(prgrm, elt, &tgtFn)
// 			} else {
// 				tgtExprAtomicOp.Outputs[tgtArgIndex] = readArgAff(prgrm, elt, &tgtFn)
// 			}
// 		case "strct":

// 		case "prgrm":

// 		}
// 	case "expr":
// 		if expr, err := tgtFn.GetExpressionByLabel(prgrm, elt); err == nil {
// 			_ = expr
// 			switch tgtElt {
// 			case "arg":

// 			case "fn":

// 			case "prgrm":

// 			}
// 		} else {
// 			panic(err)
// 		}
// 	case "fn":
// 		if fn, err := tgtPkg.GetFunction(prgrm, elt); err == nil {
// 			_ = fn
// 			switch tgtElt {
// 			case "arg":

// 			case "expr":

// 			case "Pkg":

// 			case "prgrm":

// 			}
// 		} else {
// 			panic(err)
// 		}
// 	case "strct":
// 		switch tgtElt {
// 		case "arg":

// 		case "fn":

// 		case "Pkg":

// 		}
// 	case "Pkg":
// 		if pkg, err := prgrm.GetPackage(elt); err == nil {
// 			_ = pkg
// 			switch tgtElt {
// 			case "Pkg":

// 			}
// 		} else {
// 			panic(err)
// 		}
// 		// case "prgrm":
// 		// 	switch tgtElt {
// 		// 	case "prgrm":
// 		// 		affs = append(affs, "Run program")
// 		// 	}
// 	}

// 	// returning to previous state
// 	prgrm.CurrentPackage = prevPkgIdx
// 	prevPkg.CurrentFunction = prevFnIdx
// 	prevFn.CurrentExpression = prevExpr
// }

// func opAffRequest(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
// 	inp1, inp2, inp3 := inputs[0].Arg, inputs[1].Arg, inputs[2].Arg

// 	call := prgrm.GetCurrentCall()
// 	expr := call.Operator.Expressions[call.Line]
// 	fp := inputs[0].FramePointer

// 	prevPkgIdx := prgrm.CurrentPackage
// 	prevPkg, err := prgrm.GetPackageFromArray(prevPkgIdx)
// 	if err != nil {
// 		panic(err)
// 	}

// 	prevFnIdx := prevPkg.CurrentFunction
// 	prevFn, err := prgrm.GetFunctionFromArray(prevFnIdx)
// 	if err != nil {
// 		panic(err)
// 	}
// 	prevExpr := prevFn.CurrentExpression

// 	var tgtPkg = ast.CXPackage(*prevPkg)

// 	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
// 	if err != nil {
// 		panic(err)
// 	}

// 	cxAtomicOpFunction, err := prgrm.GetFunctionFromArray(cxAtomicOp.Function)
// 	if err != nil {
// 		panic(err)
// 	}

// 	var tgtFn = ast.CXFunction(*cxAtomicOpFunction)
// 	var tgtExpr = ast.CXExpression(*prevExpr)

// 	// processing the target
// 	var tgtElt string
// 	var tgtArgType string
// 	var tgtArgIndex int

// 	getTarget(prgrm, inp3, fp, &tgtElt, &tgtArgType, &tgtArgIndex, &tgtPkg, &tgtFn, &tgtExpr)

// 	tgtExprAtomicOp, _, err := prgrm.GetOperation(&tgtExpr)
// 	if err != nil {
// 		panic(err)
// 	}

// 	// var affs []string

// 	elts := GetInferActions(prgrm, inp1, fp)
// 	eltIdx := types.Read_ptr(prgrm.Memory, ast.GetFinalOffset(prgrm, fp, inp2))
// 	eltType := elts[eltIdx*2]
// 	elt := elts[eltIdx*2+1]

// 	switch eltType {
// 	case "arg":
// 		switch tgtElt {
// 		case "arg":
// 			if tgtArgType == "inp" {
// 				// tgtExpr.ProgramInput[tgtArgIndex] = readArgAff(elt, &tgtFn)
// 				*readArgAff(prgrm, elt, &tgtFn) = *tgtExprAtomicOp.Inputs[tgtArgIndex]
// 			} else {
// 				// tgtExpr.ProgramOutput[tgtArgIndex] = readArgAff(elt, &tgtFn)
// 				*readArgAff(prgrm, elt, &tgtFn) = *tgtExprAtomicOp.Outputs[tgtArgIndex]
// 			}
// 		case "strct":

// 		case "prgrm":
// 			fmt.Println(ast.GetPrintableValue(prgrm, fp, readArgAff(prgrm, elt, &tgtFn)))
// 		}
// 	case "expr":
// 		if expr, err := tgtFn.GetExpressionByLabel(prgrm, elt); err == nil {
// 			_ = expr
// 			switch tgtElt {
// 			case "arg":

// 			case "fn":

// 			case "prgrm":

// 			}
// 		} else {
// 			panic(err)
// 		}
// 	case "fn":
// 		fn := ast.Natives[ast.OpCodes[elt]]
// 		if fn == nil {
// 			var err error
// 			fn, err = tgtPkg.GetFunction(prgrm, elt)
// 			if err != nil {
// 				panic(err)
// 			}
// 		}
// 		_ = fn
// 		switch tgtElt {
// 		case "arg":

// 		case "expr":

// 		case "Pkg":

// 		case "prgrm":

// 		}
// 	case "strct":
// 		switch tgtElt {
// 		case "arg":
// 			if tgtArgType == "inp" {
// 				// tgtExpr.ProgramInput[tgtArgIndex] = readArgAff(elt, &tgtFn)
// 				readStrctAff(prgrm, elt, &tgtPkg).AddField(tgtExprAtomicOp.Inputs[tgtArgIndex])
// 			} else {
// 				// tgtExpr.ProgramOutput[tgtArgIndex] = readArgAff(elt, &tgtFn)
// 				readStrctAff(prgrm, elt, &tgtPkg).AddField(tgtExprAtomicOp.Outputs[tgtArgIndex])
// 			}
// 		case "fn":

// 		case "Pkg":

// 		}
// 	case "Pkg":
// 		if pkg, err := prgrm.GetPackage(elt); err == nil {
// 			_ = pkg
// 			switch tgtElt {
// 			case "Pkg":

// 			}
// 		} else {
// 			panic(err)
// 		}
// 	case "prgrm":
// 		switch tgtElt {
// 		case "arg":
// 			if tgtArgType == "inp" {
// 				fmt.Println(ast.GetPrintableValue(prgrm, fp, tgtExprAtomicOp.Inputs[tgtArgIndex]))
// 			} else {
// 				fmt.Println(ast.GetPrintableValue(prgrm, fp, tgtExprAtomicOp.Outputs[tgtArgIndex]))
// 			}
// 		case "prgrm":
// 			// affs = append(affs, "Run program")
// 		}
// 	}

// 	// returning to previous state
// 	prgrm.CurrentPackage = prevPkgIdx
// 	prevPkg.CurrentFunction = prevFnIdx
// 	prevFn.CurrentExpression = prevExpr
// }

// GCREACH: allocates query strs and objects, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-080: B, an affordance query runs compiler code by name, #412.
// VIOLATION VMALLOC-019: an affordance query runs compiler code, #407.
func OpAffQuery(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := call.Operator.Expressions[call.Line]
	fp := inputs[0].FramePointer

	out1Offset := ast.GetFinalOffset(prgrm, fp, nil, outputs[0].TypeSignature)

	var affOffset types.Pointer

	var cmd string
	for _, rule := range GetInferActions(prgrm, inputs[0].TypeSignature, fp) {
		switch rule {
		case "filter":
			cmd = "filter"
		case "sort":
			cmd = "sort"
		default:
			switch cmd {
			case "filter":
				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S1290.
				inp1Pkg, err := prgrm.GetPackageFromArray(inputs[0].TypeSignature.Package)
				if err != nil {
					panic(err)
				}
				if fn, err := affFunction(prgrm, inp1Pkg, rule); err == nil {

					// arg keyword
					// argB := encoder.Serialize("arg")
					// argOffset := AllocateSeq(len(argB))
					// WriteMemory(argOffset, argB)
					argOffset := ast.AllocateStr(prgrm, "arg")
					var argOffsetB [4]byte
					types.Write_ptr(argOffsetB[:], 0, argOffset)

					// expr keyword
					// exprB := encoder.Serialize("expr")
					// exprOffset := AllocateSeq(len(exprB))
					// WriteMemory(exprOffset, exprB)
					exprOffset := ast.AllocateStr(prgrm, "expr")
					var exprOffsetB [4]byte
					types.Write_ptr(exprOffsetB[:], 0, exprOffset)

					// fn keyword
					// fnB := encoder.Serialize("fn")
					// fnOffset := AllocateSeq(len(fnB))
					// WriteMemory(fnOffset, fnB)
					fnOffset := ast.AllocateStr(prgrm, "fn")
					var fnOffsetB [4]byte
					types.Write_ptr(fnOffsetB[:], 0, fnOffset)

					// strct keyword
					// strctB := encoder.Serialize("strct")
					// strctOffset := AllocateSeq(len(strctB))
					// WriteMemory(strctOffset, strctB)
					strctOffset := ast.AllocateStr(prgrm, "strct")
					var strctOffsetB [4]byte
					types.Write_ptr(strctOffsetB[:], 0, strctOffset)

					// caller keyword
					// callerB := encoder.Serialize("caller")
					// callerOffset := AllocateSeq(len(callerB))
					// WriteMemory(callerOffset, callerB)
					callerOffset := ast.AllocateStr(prgrm, "caller")
					var callerOffsetB [4]byte
					types.Write_ptr(callerOffsetB[:], 0, callerOffset)

					// program keyword
					// prgrmB := encoder.Serialize("prgrm")
					// prgrmOffset := AllocateSeq(len(prgrmB))
					// WriteMemory(prgrmOffset, prgrmB)
					prgrmOffset := ast.AllocateStr(prgrm, "prgrm")
					var prgrmOffsetB [4]byte
					types.Write_ptr(prgrmOffsetB[:], 0, prgrmOffset)

					fnInputs := fn.GetInputs(prgrm)
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S1291.
					fnInputTypeSig := prgrm.GetCXTypeSignatureFromArray(fnInputs[0])
					var predInp *ast.CXArgument
					if fnInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1292.
						predInp = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fnInputTypeSig.Meta))
					} else {
						panic("type is not type cx argument deprecate\n\n")
					}

					if predInp.Type == types.STRUCT {
						if predInp.StructType != nil {
							switch prgrm.CXProgramNameText(predInp.StructType.Name) {
							case "Argument":
								QueryArgument(prgrm, fn, &expr, argOffsetB[:], &affOffset)
							case "Expression":
								QueryExpressions(prgrm, fn, &expr, exprOffsetB[:], &affOffset)
							case "Function":
								QueryFunction(prgrm, fn, &expr, fnOffsetB[:], &affOffset)
							case "Structure":
								QueryStructure(prgrm, fn, &expr, strctOffsetB[:], &affOffset)
							case "Caller":
								QueryCaller(prgrm, fn, &expr, callerOffsetB[:], &affOffset)
							case "Program":
								QueryProgram(prgrm, fn, &expr, prgrmOffsetB[:], &affOffset)
							}
						}
					}

				} else {
					panic(err)
				}
			case "sort":

			}
		}
	}

	types.Write_ptr(prgrm.Memory, out1Offset, affOffset)
}

// affPackage finds the package named by text. The pool is frozen at run
// time, so a name that was never interned names nothing.
func affPackage(prgrm *ast.CXProgram, text string) (*ast.CXPackage, error) {
	gid, found := prgrm.CXProgramNameFind(text)
	if !found {
		return nil, fmt.Errorf("package '%s' not found", text)
	}
	return prgrm.GetPackage(gid)
}

// affFunction finds the function named by text in pkg or its imports.
// VIOLATION STRHOT-317: B, an affordance query runs compiler code by name, #412.
func affFunction(prgrm *ast.CXProgram, pkg *ast.CXPackage, text string) (*ast.CXFunction, error) {
	gid, found := prgrm.CXProgramNameFind(text)
	if !found {
		return nil, fmt.Errorf("function '%s' not found in package '%s' or its imports", text, prgrm.CXProgramNameText(pkg.Name))
	}
	return pkg.GetFunction(prgrm, gid)
}
