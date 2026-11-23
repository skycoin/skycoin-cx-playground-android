package ast

import (
	"errors"
	"fmt"

	"github.com/skycoin/skycoin-cx/t0-types"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

type CXFunctionIndex int

// IsBuiltIn determines if opcode is not 0
// True if the function is native to CX, e.g. int32.add()
func (cxf CXFunction) IsBuiltIn() bool {
	return cxf.AtomicOPCode != 0
}

func (cxf CXFunction) IsAtomic() bool {
	return cxf.AtomicOPCode != 0
}

// MakeFunction creates an empty function.
// Later, parameters and contents can be added.
// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
// Now a constructor, site S0133.
func MakeFunction(prgrm *CXProgram, name stringpool.StringGid, fileName SourceFileGid, fileLine int) *CXFunction {
	// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
	// Now a heap literal, site S0134.
	return &CXFunction{
		Name:     name,
		Index:    -1,
		FileName: fileName,
		FileLine: fileLine,
		// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
		// Now a heap literal, site S0135.
		Inputs: &CXStruct{
			Name: prgrm.CXProgramNameIntern(prgrm.CXProgramNameText(name) + "_Input"),
		},
		LocalVariables: []stringpool.StringGid{},
	}
}

// ----------------------------------------------------------------
//                             `CXFunction` Getters

// GetExpressions is not used
func (fn *CXFunction) GetExpressions(prgrm *CXProgram) ([]CXExpression, error) {
	if fn.Expressions == nil {
		return nil, fmt.Errorf("function '%s' has no expressions", prgrm.CXProgramNameText(fn.Name))
	}

	return fn.Expressions, nil

}

// GetExpressionByLabel
func (fn *CXFunction) GetExpressionByLabel(prgrm *CXProgram, lbl string) (*CXExpression, error) {
	if fn.Expressions == nil {
		return nil, fmt.Errorf("function '%s' has no expressions", prgrm.CXProgramNameText(fn.Name))
	}

	for _, expr := range fn.Expressions {
		if prgrm.CXProgramNameText(expr.GetLabel(prgrm)) == lbl {
			return &expr, nil
		}
	}
	return nil, fmt.Errorf("expression '%s' not found in function '%s'", lbl, prgrm.CXProgramNameText(fn.Name))
}

// GetExpressionByLine ...
func (fn *CXFunction) GetExpressionByLine(prgrm *CXProgram, line int) (*CXExpression, error) {
	if fn.Expressions == nil {
		return nil, fmt.Errorf("function '%s' has no expressions", prgrm.CXProgramNameText(fn.Name))
	}

	if line > len(fn.Expressions) {
		return nil, fmt.Errorf("expression line number '%d' exceeds number of expressions in function '%s'", line, prgrm.CXProgramNameText(fn.Name))
	}

	return &fn.Expressions[line], nil
}

// ----------------------------------------------------------------
//                     `CXFunction` Member handling

// AddInput adds param to the inputs of fn. It takes a managed param, which it
// releases when the signature does not name it or the input exists.
func (fn *CXFunction) AddInput(prgrm *CXProgram, param *CXArgument) *CXFunction {
	fnInputs := fn.GetInputs(prgrm)
	for _, inputIdx := range fnInputs {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0136.
		input := prgrm.GetCXTypeSignatureFromArray(inputIdx)
		if input.Name == param.Name {
			prgrm.ReleaseCXArgBuilder(param)
			return fn
		}
	}

	param.Package = fn.Package
	newFieldIdx := AllocCXTypeSignatureOfCXArgBuilder(prgrm, param)

	if fn.Inputs == nil {
		// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
		// Now a heap literal, site S0137.
		fn.Inputs = &CXStruct{}
	}

	fn.Inputs.AddField_TypeSignature(prgrm, newFieldIdx)

	return fn
}

func (fn *CXFunction) GetInputs(prgrm *CXProgram) []CXTypeSignatureIndex {
	if fn == nil || fn.Inputs == nil {
		return []CXTypeSignatureIndex{}
	}

	return fn.Inputs.Fields
}

// RemoveInput ...
// func (fn *CXFunction) RemoveInput(prgrm *CXProgram, inpName string) {
// 	if len(fn.Inputs) > 0 {
// 		lenInps := len(fn.Inputs)
// 		for i, inpIdx := range fn.Inputs {
// 			inp := prgrm.GetCXArgFromArray(inpIdx)
// 			if inp.Name == inpName {
// 				if i == lenInps {
// 					fn.Inputs = fn.Inputs[:len(fn.Inputs)-1]
// 				} else {
// 					fn.Inputs = append(fn.Inputs[:i], fn.Inputs[i+1:]...)
// 				}
// 				break
// 			}
// 		}
// 	}
// }

// AddOutput adds param to the outputs of fn like AddInput.
func (fn *CXFunction) AddOutput(prgrm *CXProgram, param *CXArgument) *CXFunction {
	fnOutputs := fn.GetOutputs(prgrm)
	for _, outputIdx := range fnOutputs {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0139.
		output := prgrm.GetCXTypeSignatureFromArray(outputIdx)
		if output.Name == param.Name {
			prgrm.ReleaseCXArgBuilder(param)
			return fn
		}
	}

	param.Package = fn.Package

	// An array output, or a pointer to one, stays a CXArgument, the call,
	// return and assignment paths have no array signature case.
	var newFieldIdx CXTypeSignatureIndex
	if IsTypeArrayAtomic(param) || IsTypePointerArrayAtomic(param) {
		newFieldIdx = AllocCXTypeSignatureOfCXArgDeprecate(prgrm, param)
	} else {
		newFieldIdx = AllocCXTypeSignatureOfCXArgBuilder(prgrm, param)
	}

	if fn.Outputs == nil {
		// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
		// Now a heap literal, site S0140.
		fn.Outputs = &CXStruct{}
	}

	fn.Outputs.AddField_TypeSignature(prgrm, newFieldIdx)

	return fn
}

func (fn *CXFunction) GetOutputs(prgrm *CXProgram) []CXTypeSignatureIndex {
	if fn == nil || fn.Outputs == nil {
		return []CXTypeSignatureIndex{}
	}

	return fn.Outputs.Fields
}

// RemoveOutput ...
// func (fn *CXFunction) RemoveOutput(prgrm *CXProgram, outName string) {
// 	if len(fn.Outputs) > 0 {
// 		lenOuts := len(fn.Outputs)
// 		for i, outIdx := range fn.Outputs {
// 			out := prgrm.GetCXArgFromArray(outIdx)
// 			if out.Name == outName {
// 				if i == lenOuts {
// 					fn.Outputs = fn.Outputs[:len(fn.Outputs)-1]
// 				} else {
// 					fn.Outputs = append(fn.Outputs[:i], fn.Outputs[i+1:]...)
// 				}
// 				break
// 			}
// 		}
// 	}
// }

// AddExpression ...
func (fn *CXFunction) AddExpression(prgrm *CXProgram, expr *CXExpression) *CXFunction {
	if expr.Type == CX_ATOMIC_OPERATOR {
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0142.
		cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		cxAtomicOp.Package = fn.Package
		cxAtomicOp.Function = CXFunctionIndex(fn.Index)
	}

	fn.Expressions = append(fn.Expressions, *expr)
	fn.LineCount++
	return fn
}

func (fn *CXFunction) AddExpressionByLineNumber(prgrm *CXProgram, expr *CXExpression, line int) *CXFunction {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0143.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	cxAtomicOp.Package = fn.Package
	cxAtomicOp.Function = CXFunctionIndex(fn.Index)

	lenExprs := len(fn.Expressions)
	if lenExprs == line {
		fn.Expressions = append(fn.Expressions, *expr)
	} else {
		// OBJMODEL: state.CXExpressionMng.CXExpressionRelease(gid), phase 4 #346.
		// Now a dropped object, site S0144.
		fn.Expressions = append(fn.Expressions[:line+1], fn.Expressions[line:]...)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now an object copy, site S0145.
		fn.Expressions[line] = *expr
	}

	fn.LineCount++
	return fn

}

// RemoveExpression ...
func (fn *CXFunction) RemoveExpression(line int) {
	if len(fn.Expressions) > 0 {
		lenExprs := len(fn.Expressions)
		if line >= lenExprs-1 || line < 0 {
			// OBJMODEL: state.CXExpressionMng.CXExpressionRelease(gid), phase 4 #346.
			// Now a dropped object, site S0146.
			fn.Expressions = fn.Expressions[:len(fn.Expressions)-1]
		} else {
			// OBJMODEL: state.CXExpressionMng.CXExpressionRelease(gid), phase 4 #346.
			// Now a dropped object, site S0147.
			fn.Expressions = append(fn.Expressions[:line], fn.Expressions[line+1:]...)
		}
	}
}

func (fn *CXFunction) AddLocalVariableName(name stringpool.StringGid) error {
	fn.LocalVariables = append(fn.LocalVariables, name)

	return nil
}

func (fn *CXFunction) IsLocalVariable(name stringpool.StringGid) bool {
	for _, localVar := range fn.LocalVariables {
		if name == localVar {
			return true
		}
	}

	return false
}

func (fn *CXFunction) RemoveLocalVariableFromArray(name stringpool.StringGid) error {
	for idx, localVar := range fn.LocalVariables {
		if name == localVar {
			fn.LocalVariables = append(fn.LocalVariables[:idx], fn.LocalVariables[idx+1:]...)
			return nil
		}
	}

	return nil
}

// ----------------------------------------------------------------
//                             `CXFunction` Selectors

// MakeAtomicOperatorExpression ...
// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
// Now a constructor, site S0148.
func MakeAtomicOperatorExpression(prgrm *CXProgram, op *CXNativeFunction) *CXExpression {
	var opIdx CXFunctionIndex = -1
	if op != nil {
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now an arena insert, site S0149.
		opIdx = prgrm.AddNativeFunctionInArray(op)
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
	// Now a heap literal, site S0150 S0151.
	index := prgrm.AddCXAtomicOp(&CXAtomicOperator{
		Operator: opIdx,
		// Function: -1,
	})

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a heap literal, site S0152.
	return &CXExpression{
		Index: index,
		Type:  CX_ATOMIC_OPERATOR,
	}
}

// MakeCXLineExpression ...
// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
// Now a constructor, site S0153.
func MakeCXLineExpression(prgrm *CXProgram, fileName SourceFileGid, lineNo int, lineStr stringpool.StringGid) *CXExpression {
	lineGid := prgrm.CxIrState.CXLineMng.CXLineAlloc()
	line := prgrm.CxIrState.CXLineMng.CXLineGet(lineGid)
	line.FileName = fileName
	line.LineNumber = lineNo
	line.LineColumn = prgrm.CXProgramCompileErrorColumn(fileName, lineNo)
	line.LineStr = lineStr
	index := int(objmodel.GidIndex(int64(lineGid)))

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a heap literal, site S0156.
	return &CXExpression{
		Index: index,
		Type:  CX_LINE,
	}
}

// SelectExpression ...
func (fn *CXFunction) SelectExpression(line int) (*CXExpression, error) {
	// prgrmStep := &CXProgramStep{
	// 	Action: func(cxt *CXProgram) {
	// 		if mod, err := cxt.GetCurrentPackage(); err == nil {
	// 			if fn, err := mod.GetCurrentFunction(); err == nil {
	// 				fn.SelectExpression(line)
	// 			}
	// 		}
	// 	},
	// }
	// saveProgramStep(prgrmStep, fn.Context)
	if len(fn.Expressions) == 0 {
		return nil, errors.New("There are no expressions in this function")
	}

	if line >= len(fn.Expressions) {
		line = len(fn.Expressions) - 1
	}

	if line < 0 {
		line = 0
	}

	expr := fn.Expressions[line]

	return &expr, nil
}

// CXFunctionEscape says where a function may keep the address an input holds.
type CXFunctionEscape uint8

const (
	// CXFunctionEscapeNone keeps it only for the call.
	CXFunctionEscapeNone CXFunctionEscape = iota
	// CXFunctionEscapeResult may return it in an output.
	CXFunctionEscapeResult
	// CXFunctionEscapeHeap may keep it after the call.
	CXFunctionEscapeHeap
)

// CXFunctionSlot is a local of a function that holds a pointer, slice or str,
// with its declared type. The GC walks it as a root by its GCType.
type CXFunctionSlot struct {
	// CXFunctionSlotOffset is the offset of the local, flagged when it escaped.
	CXFunctionSlotOffset types.Pointer
	// CXFunctionSlotLengths are the lengths of the arrays and slices of the
	// local, outermost first, as CXArgument.Lengths.
	CXFunctionSlotLengths []types.Pointer
	// CXFunctionSlotType is the base type of the local, types.STR for a str.
	CXFunctionSlotType types.Code
	// CXFunctionSlotSpecifiers are the declaration specifiers of the local.
	CXFunctionSlotSpecifiers []int
	// CXFunctionSlotStruct is the struct the local is made of, or nil.
	CXFunctionSlotStruct *CXStruct
}
