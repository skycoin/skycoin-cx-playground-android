package printer

import (
	"fmt"

	"github.com/skycoin/skycoin-cx/t0-types"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// printerStackValueBadIndex is printed for a value whose index is out of range.
const printerStackValueBadIndex = "<index out of range>"

func printerStackValueHeader(fileName string, fileLine int) string {
	return fmt.Sprintf("%s:%d", fileName, fileLine)
}

// printerStackValuePrintable is ast.GetPrintableValue for the call stack of a runtime
// error. A value whose index is bad, often the one that failed, prints as a
// placeholder instead of panicking again.
func printerStackValuePrintable(prgrm *ast.CXProgram, fp types.Pointer, argTypeSig *ast.CXTypeSignature) string {
	if offset, valid := ast.FinalOffsetChecked(prgrm, fp, argTypeSig); !valid || int(offset) >= len(prgrm.Memory) {
		return printerStackValueBadIndex
	}
	return ast.GetPrintableValue(prgrm, fp, argTypeSig)
}

// PrinterStackPrint prints the CX call stack of cxprogram from the current
// call down to the entry point, with the values of its arguments.
func PrinterStackPrint(cxprogram *ast.CXProgram) {
	fmt.Println()
	fmt.Println("===Callstack===")

	// we're going backwards in the stack
	fp := cxprogram.Stack.Pointer

	for c := cxprogram.CallCounter; c != types.InvalidPointer; c-- {
		op := cxprogram.CallStack[c].Operator
		fp -= op.Size

		var dupNames []string

		fmt.Printf(">>> %s()\n", cxprogram.CXProgramNameText(op.Name))

		opInputs := op.GetInputs(cxprogram)
		for _, inputIdx := range opInputs {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0465.
			input := cxprogram.GetCXTypeSignatureFromArray(inputIdx)

			fmt.Println("ProgramInput")
			fmt.Printf("\t%s : %s() : %s\n", printerStackValueHeader(ast.CXArgumentDebugFileLine(cxprogram, input.ArgDetails)), cxprogram.CXProgramNameText(op.Name), printerStackValuePrintable(cxprogram, fp, input))

			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S0466.
			inpPkg, err := cxprogram.GetPackageFromArray(input.Package)
			if err != nil {
				panic(err)
			}
			dupNames = append(dupNames, cxprogram.CXProgramNameText(inpPkg.Name)+cxprogram.CXProgramNameText(input.Name))
		}

		opOutputs := op.GetOutputs(cxprogram)
		for _, outputIdx := range opOutputs {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0467.
			output := cxprogram.GetCXTypeSignatureFromArray(outputIdx)

			fmt.Println("ProgramOutput")
			fmt.Printf("\t%s : %s() : %s\n", printerStackValueHeader(ast.CXArgumentDebugFileLine(cxprogram, output.ArgDetails)), cxprogram.CXProgramNameText(op.Name), printerStackValuePrintable(cxprogram, fp, output))

			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S0468.
			outPkg, err := cxprogram.GetPackageFromArray(output.Package)
			if err != nil {
				panic(err)
			}
			dupNames = append(dupNames, cxprogram.CXProgramNameText(outPkg.Name)+cxprogram.CXProgramNameText(output.Name))
		}

		// fmt.Println("Expressions")
		exprs := ""
		for _, expr := range op.Expressions {
			// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
			// Now an arena getter, site S0469.
			cxAtomicOp, err := cxprogram.GetCXAtomicOp(expr.Index)
			if err != nil {
				panic(err)
			}

			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now an arena getter, site S0470.
			cxAtomicOpOperator := cxprogram.GetFunctionFromArray(cxAtomicOp.Operator)
			for _, inputIdx := range cxAtomicOp.GetInputs(cxprogram) {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0471.
				input := cxprogram.GetCXTypeSignatureFromArray(inputIdx)

				if input.Name == stringpool.StringGidNil || cxAtomicOpOperator == nil {
					continue
				}

				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S0472.
				inpPkg, err := cxprogram.GetPackageFromArray(input.Package)
				if err != nil {
					panic(err)
				}
				var dup bool
				for _, name := range dupNames {
					if name == cxprogram.CXProgramNameText(inpPkg.Name)+cxprogram.CXProgramNameText(input.Name) {
						dup = true
						break
					}
				}
				if dup {
					continue
				}

				// fmt.Println("\t", inp.Name, "\t", ":", "\t", GetPrintableValue(fp, inp))
				// exprs += fmt.Sprintln("\t", printerStackValueHeader(inp.FileName, inp.FileLine), "\t", ":", "\t", GetPrintableValue(fp, inp))
				exprs += fmt.Sprintf("\t%s : %s() : %s\n", printerStackValueHeader(ast.CXArgumentDebugFileLine(cxprogram, input.ArgDetails)), cxAtomicOp.GetOperatorName(cxprogram), printerStackValuePrintable(cxprogram, fp, input))

				dupNames = append(dupNames, cxprogram.CXProgramNameText(inpPkg.Name)+cxprogram.CXProgramNameText(input.Name))
			}

			for _, outputIdx := range cxAtomicOp.GetOutputs(cxprogram) {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0473.
				output := cxprogram.GetCXTypeSignatureFromArray(outputIdx)

				if output.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					if output.Name == stringpool.StringGidNil || cxAtomicOpOperator == nil {
						continue
					}

					// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
					// Now an arena getter, site S0474.
					outPkg, err := cxprogram.GetPackageFromArray(output.Package)
					if err != nil {
						panic(err)
					}
					var dup bool
					for _, name := range dupNames {
						if name == cxprogram.CXProgramNameText(outPkg.Name)+cxprogram.CXProgramNameText(output.Name) {
							dup = true
							break
						}
					}
					if dup {
						continue
					}

					// fmt.Println("\t", out.Name, "\t", ":", "\t", GetPrintableValue(fp, out))
					// exprs += fmt.Sprintln("\t", printerStackValueHeader(out.FileName, out.FileLine), ":", GetPrintableValue(fp, out))

					exprs += fmt.Sprintf("\t%s : %s() : %s\n", printerStackValueHeader(ast.CXArgumentDebugFileLine(cxprogram, output.ArgDetails)), cxAtomicOp.GetOperatorName(cxprogram), printerStackValuePrintable(cxprogram, fp, output))

					dupNames = append(dupNames, cxprogram.CXProgramNameText(outPkg.Name)+cxprogram.CXProgramNameText(output.Name))
				} else if output.Type == ast.TYPE_ATOMIC || output.Type == ast.TYPE_POINTER_ATOMIC || output.Type == ast.TYPE_ARRAY_ATOMIC || output.Type == ast.TYPE_POINTER_ARRAY_ATOMIC || output.Type == ast.TYPE_SLICE_ATOMIC || output.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
					if output.Name == stringpool.StringGidNil || cxAtomicOpOperator == nil {
						continue
					}

					// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
					// Now an arena getter, site S0475.
					outputPkg, err := cxprogram.GetPackageFromArray(output.Package)
					if err != nil {
						panic(err)
					}

					var dup bool
					for _, name := range dupNames {
						if name == cxprogram.CXProgramNameText(outputPkg.Name)+cxprogram.CXProgramNameText(output.Name) {
							dup = true
							break
						}
					}
					if dup {
						continue
					}

					// TODO: Make GetPrintableValue() and other functions receive CXTypeSignature instead of CXArgument
					exprs += fmt.Sprintf("\t%s : %s() : %s\n", printerStackValueHeader(ast.CXArgumentDebugFileLine(cxprogram, output.ArgDetails)), cxAtomicOp.GetOperatorName(cxprogram), printerStackValuePrintable(cxprogram, fp, output))

					dupNames = append(dupNames, cxprogram.CXProgramNameText(outputPkg.Name)+cxprogram.CXProgramNameText(output.Name))

				}

			}
		}

		if len(exprs) > 0 {
			fmt.Println("Expressions\n", exprs)
		}
	}
}
