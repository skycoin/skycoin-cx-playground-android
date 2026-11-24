package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	//"fmt"
)

func popStack(prgrm *CXProgram, call *CXCall) error {
	// going back to the previous call
	prgrm.CallCounter--
	if !prgrm.CallCounter.IsValid() {
		// then the program finished
		prgrm.Terminated = true
		return nil
	}

	// copying the outputs to the previous stack frame
	returnAddr := &prgrm.CallStack[prgrm.CallCounter]
	returnOp := returnAddr.Operator
	returnLine := returnAddr.Line
	returnFP := returnAddr.FramePointer
	fp := call.FramePointer

	expr := returnOp.Expressions[returnLine]

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0097.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		return err
	}

	cxAtomicOpOutputs := cxAtomicOp.GetOutputs(prgrm)
	lenOuts := len(cxAtomicOpOutputs)
	callOperatorOutputs := call.Operator.GetOutputs(prgrm)
	for i, outputIdx := range callOperatorOutputs {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0098.
		output := prgrm.GetCXTypeSignatureFromArray(outputIdx)
		// Continuing if there is no receiving variable available.
		if i >= lenOuts {
			continue
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0099.
		cxAtomicOpOutput := prgrm.GetCXTypeSignatureFromArray(cxAtomicOpOutputs[i])
		types.WriteSlice_byte(prgrm.Memory, GetFinalOffset(prgrm, returnFP, nil, cxAtomicOpOutput),
			types.GetSlice_byte(prgrm.Memory, GetFinalOffset(prgrm, fp, nil, output), output.GetSize(prgrm, false)))
	}

	// return the stack pointer to its previous state
	prgrm.Stack.Pointer = fp
	// we'll now execute the next command
	prgrm.CallStack[prgrm.CallCounter].Line++
	// calling the actual command
	// prgrm.CallStack[prgrm.CallCounter].Call(prgrm)
	return nil
}

func wipeDeclarationMemory(prgrm *CXProgram, cxAtomicOp *CXAtomicOperator) error {
	newCall := &prgrm.CallStack[prgrm.CallCounter]
	newFP := newCall.FramePointer
	cxAtomicOpOutputs := cxAtomicOp.GetOutputs(prgrm)
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0101.
	cxAtomicOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(cxAtomicOpOutputs[0])
	size := cxAtomicOutputTypeSig.GetSize(prgrm, false)
	var offset types.Pointer
	if cxAtomicOutputTypeSig.Type == TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1630.
		offset = prgrm.GetCXArg(CXArgumentIndex(cxAtomicOutputTypeSig.Meta)).Offset
	} else if cxAtomicOutputTypeSig.Type == TYPE_ATOMIC || cxAtomicOutputTypeSig.Type == TYPE_POINTER_ATOMIC {
		offset = cxAtomicOutputTypeSig.Offset
	} else if cxAtomicOutputTypeSig.Type == TYPE_ARRAY_ATOMIC {
		offset = cxAtomicOutputTypeSig.Offset
	} else if cxAtomicOutputTypeSig.Type == TYPE_POINTER_ARRAY_ATOMIC {
		offset = cxAtomicOutputTypeSig.Offset
	} else if cxAtomicOutputTypeSig.Type == TYPE_SLICE_ATOMIC {
		offset = cxAtomicOutputTypeSig.Offset
	} else if cxAtomicOutputTypeSig.Type == TYPE_POINTER_SLICE_ATOMIC {
		offset = cxAtomicOutputTypeSig.Offset
	} else if cxAtomicOutputTypeSig.Type == TYPE_STRUCT {
		offset = cxAtomicOutputTypeSig.Offset
	} else if cxAtomicOutputTypeSig.Type == TYPE_POINTER_STRUCT {
		offset = cxAtomicOutputTypeSig.Offset
	} else {
		panic("type is not known")
	}

	if OffsetEscaped(offset) {
		// GCREACH: allocates the object of an escaped local, which adds to
		// the GC pressure, see docs/GarbageCollector.md.
		EscapedAllocate(prgrm, newFP, offset, escapedSize(prgrm, cxAtomicOutputTypeSig))
		return nil
	}
	for c := types.Pointer(0); c < size; c++ {
		prgrm.Memory[newFP+offset+c] = 0
	}

	return nil
}

func processBuiltInOperators(prgrm *CXProgram, expr *CXExpression, cxAtomicOp *CXAtomicOperator, cxAtomicOpOperator *CXFunction, globalInputs *[]CXValue, globalOutputs *[]CXValue, fp types.Pointer) error {
	inputs := cxAtomicOp.GetInputs(prgrm)
	inputCount := len(inputs)
	// VIOLATION VMALLOC-002: grows the operand buffers in a Callback loop only,
	// RunBuffersPrepare sizes those of the run, #407.
	if inputCount > len(*globalInputs) {
		*globalInputs = make([]CXValue, inputCount)
	}
	inputValues := (*globalInputs)[:inputCount]

	outputs := cxAtomicOp.GetOutputs(prgrm)
	outputCount := len(outputs)
	if outputCount > len(*globalOutputs) {
		*globalOutputs = make([]CXValue, outputCount)
	}
	outputValues := (*globalOutputs)[:outputCount]

	argIndex := 0
	for inputIndex := 0; inputIndex < inputCount; inputIndex++ {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0104.
		inputTypeSignature := prgrm.GetCXTypeSignatureFromArray(inputs[inputIndex])
		value := &inputValues[inputIndex]
		value.TypeSignature = inputTypeSignature
		value.Size = inputTypeSignature.GetSize(prgrm, false)
		offset := GetFinalOffset(prgrm, fp, nil, inputTypeSignature)
		value.Offset = offset

		var input *CXArgument
		if inputTypeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0106.
			input = prgrm.GetCXArgFromArray(CXArgumentIndex(inputTypeSignature.Meta))

			value.Type = input.Type
			if input.Type == types.POINTER {
				value.Type = input.PointerTargetType
			}
		} else {
			value.Type = inputTypeSignature.GetType(prgrm)
		}

		value.FramePointer = fp
		value.Expr = expr
		argIndex++
	}

	for outputIndex := 0; outputIndex < outputCount; outputIndex++ {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0107.
		outputTypeSignature := prgrm.GetCXTypeSignatureFromArray(outputs[outputIndex])

		value := &outputValues[outputIndex]
		value.TypeSignature = outputTypeSignature
		value.Size = outputTypeSignature.GetSize(prgrm, false)
		offset := GetFinalOffset(prgrm, fp, nil, outputTypeSignature)
		value.Offset = offset

		var output *CXArgument
		if outputTypeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0109.
			output = prgrm.GetCXArgFromArray(CXArgumentIndex(outputTypeSignature.Meta))

			value.Type = output.Type
			if output.Type == types.POINTER {
				value.Type = output.PointerTargetType
			}
		} else {
			value.Type = outputTypeSignature.GetType(prgrm)
		}

		value.FramePointer = fp
		value.Expr = expr
		argIndex++
	}

	if cxAtomicOpOperator.AtomicOPCode == constants.OP_LIBRARY_CALL {
		libraryCall(prgrm, cxAtomicOpOperator, inputValues, outputValues)
	} else {
		OpcodeHandlers[cxAtomicOpOperator.AtomicOPCode](prgrm, inputValues, outputValues)
	}

	// Expr points into fn.Expressions and is valid for this step only.
	for i := range inputValues {
		inputValues[i].Expr = nil
	}
	for i := range outputValues {
		outputValues[i].Expr = nil
	}

	return nil
}

func processNonAtomicOperators(prgrm *CXProgram, cxAtomicOp *CXAtomicOperator, cxAtomicOpOperator *CXFunction, fp types.Pointer) error {
	//TODO: Is this only called for user defined functions?
	/*
	   It was not a native, so we need to create another call
	   with the current expression's operator
	*/
	newFP := prgrm.Stack.Pointer
	callPushCheck(prgrm, cxAtomicOpOperator)

	// The inputs are copied before the push, so an error in one stops the
	// caller, as it does on vm.
	for c := types.Pointer(0); c < cxAtomicOpOperator.Size; c++ {
		prgrm.Memory[newFP+c] = 0
	}

	newCallOperatorInputs := cxAtomicOpOperator.GetInputs(prgrm)
	for i, inputIdx := range cxAtomicOp.GetInputs(prgrm) {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0112.
		input := prgrm.GetCXTypeSignatureFromArray(inputIdx)
		// An atomic input is passed by its signature only.
		inpPassBy := 0
		if input.Type == TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0114.
			inpPassBy = prgrm.GetCXArgFromArray(CXArgumentIndex(input.Meta)).PassBy
		}

		var byts []byte

		finalOffset := GetFinalOffset(prgrm, fp, nil, input)
		if input.PassBy == constants.PASSBY_REFERENCE || inpPassBy == constants.PASSBY_REFERENCE {
			finalOffset = PointerOfAddress(prgrm, finalOffset)
			var finalOffsetB [types.POINTER_SIZE]byte
			types.Write_ptr(finalOffsetB[:], 0, finalOffset)
			byts = finalOffsetB[:]

		} else {
			size := input.GetSize(prgrm, false)
			byts = prgrm.Memory[finalOffset : finalOffset+size]
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0115.
		newCallOperatorInputTypeSignature := prgrm.GetCXTypeSignatureFromArray(newCallOperatorInputs[i])
		// writing inputs to new stack frame
		types.WriteSlice_byte(
			prgrm.Memory,
			GetFinalOffset(prgrm, newFP, nil, newCallOperatorInputTypeSignature),
			byts)
	}

	callPush(prgrm, cxAtomicOpOperator)
	return nil
}

// callPushCheck panics with STACK_OVERFLOW_ERROR when a call of operator
// does not fit in the call stack or the stack. The caller stays the current call.
func callPushCheck(prgrm *CXProgram, operator *CXFunction) {
	if prgrm.CallCounter+1 >= constants.CALLSTACK_SIZE || prgrm.Stack.Pointer+operator.Size > constants.STACK_SIZE {
		panic(constants.STACK_OVERFLOW_ERROR)
	}
}

// callPush makes operator the current call, with its frame at the stack
// pointer, after callPushCheck.
func callPush(prgrm *CXProgram, operator *CXFunction) {
	callPushCheck(prgrm, operator)
	prgrm.CallCounter++
	newCall := &prgrm.CallStack[prgrm.CallCounter]
	newCall.Operator = operator
	newCall.Line = 0
	newCall.FramePointer = prgrm.Stack.Pointer
	prgrm.Stack.Pointer += operator.Size
}

func (call *CXCall) Call(prgrm *CXProgram, globalInputs *[]CXValue, globalOutputs *[]CXValue) error {
	// CX is still single-threaded, so only one stack
	if call.Line >= call.Operator.LineCount {
		/*
		   popping the stack
		*/
		err := popStack(prgrm, call)
		if err != nil {
			return err
		}

		return nil
	}

	/*
	   continue with call operator's execution
	*/

	fn := call.Operator
	// A pointer, not a copy, so nothing escapes to the heap (VMALLOC-001, #407).
	expr := &fn.Expressions[call.Line]
	fp := call.FramePointer

	if expr.Type == CX_LINE {
		call.Line++
		return nil
	}
	if operationCountEnabled {
		OperationCount++
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0116.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		return err
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0117.
	cxAtomicOpOperator := prgrm.GetFunctionFromArray(cxAtomicOp.Operator)
	// if it's a native, then we just process the arguments with execNative
	//TODO: WHEN WOULD OPERATOR EVER BE NIL?
	if cxAtomicOpOperator == nil {
		// then it's a declaration
		// wiping this declaration's memory (removing garbage)
		err := wipeDeclarationMemory(prgrm, cxAtomicOp)
		if err != nil {
			return err
		}

		call.Line++
	} else if cxAtomicOpOperator.IsBuiltIn() {
		//TODO: SLICES ARE NON ATOMIC
		err := processBuiltInOperators(prgrm, expr, cxAtomicOp, cxAtomicOpOperator, globalInputs, globalOutputs, fp)
		if err != nil {
			return err
		}

		call.Line++
	} else {
		//NON-ATOMIC OPERATOR
		err := processNonAtomicOperators(prgrm, cxAtomicOp, cxAtomicOpOperator, fp)
		if err != nil {
			return err
		}
	}

	return nil
}

// prgrm.CallStack = MakeCallStack(0)
// OBJMODEL: state.CXCallMng.CXCallAlloc(), phase 4 #366.
// Now a constructor, site S0118.
func MakeCallStack(size int) []CXCall {
	// OBJMODEL: state.CXCallMng (manager storage) or []CXCallGid, phase 4 #366.
	// Now a slice of objects, site S0119.
	return make([]CXCall, 0)
	// return &CXCallStack{
	// 	Calls: make([]*CXCall, size),
	// }
}
