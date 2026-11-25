package printer

import (
	"os"
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-types"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// printerStackValueBadIndex is printed for a value whose index is out of range.
const printerStackValueBadIndex = "<index out of range>"

// printerStackHeader starts the dump.
var printerStackHeader = []byte("\n===Callstack===\n")

// printerStackValueLine appends "\tfile:line : name() : value\n" for the
// value of argTypeSig in the frame at fp. A value whose index is bad, often
// the one that failed, prints as a placeholder instead of panicking again.
func printerStackValueLine(dst []byte, prgrm *ast.CXProgram, fp types.Pointer, argTypeSig *ast.CXTypeSignature, name []byte) []byte {
	fileName, fileLine := ast.CXArgumentDebugFileLine(prgrm, argTypeSig.ArgDetails)
	dst = append(dst, '\t')
	dst = append(dst, fileName...)
	dst = append(dst, ':')
	dst = strconv.AppendInt(dst, int64(fileLine), 10)
	dst = append(dst, " : "...)
	dst = append(dst, name...)
	dst = append(dst, "() : "...)
	if offset, valid := ast.FinalOffsetChecked(prgrm, fp, argTypeSig); !valid || int(offset) >= len(prgrm.Memory) {
		dst = append(dst, printerStackValueBadIndex...)
	} else {
		dst = ast.AppendPrintableValue(dst, prgrm, fp, argTypeSig)
	}
	return append(dst, '\n')
}

// printerStackSameName reports whether a and b name the same variable of the
// same package.
func printerStackSameName(a, b *ast.CXTypeSignature) bool {
	return a.Name == b.Name && a.Package == b.Package
}

// printerStackShown reports whether the expression argument sig is printed,
// as an input when output is false.
func printerStackShown(sig *ast.CXTypeSignature, operator *ast.CXFunction, output bool) bool {
	if sig.Name == stringpool.StringGidNil || operator == nil {
		return false
	}
	if !output {
		return true
	}
	switch sig.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE, ast.TYPE_ATOMIC, ast.TYPE_POINTER_ATOMIC, ast.TYPE_ARRAY_ATOMIC,
		ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		return true
	}
	return false
}

// printerStackAtomicOp returns the atomic operator of expr and its function.
func printerStackAtomicOp(cxprogram *ast.CXProgram, expr *ast.CXExpression) (*ast.CXAtomicOperator, *ast.CXFunction) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0469.
	cxAtomicOp, err := cxprogram.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0470.
	return cxAtomicOp, cxprogram.GetFunctionFromArray(cxAtomicOp.Operator)
}

// printerStackShownBefore reports whether a variable named as sig is an
// input or output of op, or was printed for an expression argument before
// argument index of expression exprIndex, its outputs following its inputs.
// The dump prints each variable once.
func printerStackShownBefore(cxprogram *ast.CXProgram, op *ast.CXFunction, sig *ast.CXTypeSignature, exprIndex, index int) bool {
	for _, params := range [2][]ast.CXTypeSignatureIndex{op.GetInputs(cxprogram), op.GetOutputs(cxprogram)} {
		for _, paramIdx := range params {
			if printerStackSameName(cxprogram.GetCXTypeSignatureFromArray(paramIdx), sig) {
				return true
			}
		}
	}
	for e := 0; e <= exprIndex; e++ {
		cxAtomicOp, operator := printerStackAtomicOp(cxprogram, &op.Expressions[e])
		arg := 0
		for k, args := range [2][]ast.CXTypeSignatureIndex{cxAtomicOp.GetInputs(cxprogram), cxAtomicOp.GetOutputs(cxprogram)} {
			for _, argIdx := range args {
				if e == exprIndex && arg == index {
					return false
				}
				arg++
				other := cxprogram.GetCXTypeSignatureFromArray(argIdx)
				if printerStackShown(other, operator, k == 1) && printerStackSameName(other, sig) {
					return true
				}
			}
		}
	}
	return false
}

// PrinterStackPrint prints the CX call stack of cxprogram from the current
// call down to the entry point, with the values of its arguments. Each frame
// is appended to the format buffer and written at once, so the debug opcode
// allocates nothing (#407).
func PrinterStackPrint(cxprogram *ast.CXProgram) {
	os.Stdout.Write(printerStackHeader)

	// we're going backwards in the stack
	fp := cxprogram.Stack.Pointer

	for c := cxprogram.CallCounter; c != types.InvalidPointer; c-- {
		op := cxprogram.CallStack[c].Operator
		fp -= op.Size
		opName := cxprogram.CXProgramNameBytes(op.Name)

		text := cxprogram.ProgramFormatBuffer[:0]
		text = append(text, ">>> "...)
		text = append(text, opName...)
		text = append(text, "()\n"...)

		for k, params := range [2][]ast.CXTypeSignatureIndex{op.GetInputs(cxprogram), op.GetOutputs(cxprogram)} {
			for _, paramIdx := range params {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0465.
				param := cxprogram.GetCXTypeSignatureFromArray(paramIdx)
				if k == 0 {
					text = append(text, "ProgramInput\n"...)
				} else {
					text = append(text, "ProgramOutput\n"...)
				}
				text = printerStackValueLine(text, cxprogram, fp, param, opName)
				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S0466.
				if _, err := cxprogram.GetPackageFromArray(param.Package); err != nil {
					panic(err)
				}
			}
		}

		exprsStart := len(text)
		text = append(text, "Expressions\n "...)
		exprsShown := false
		for e := range op.Expressions {
			cxAtomicOp, operator := printerStackAtomicOp(cxprogram, &op.Expressions[e])
			arg := 0
			for k, args := range [2][]ast.CXTypeSignatureIndex{cxAtomicOp.GetInputs(cxprogram), cxAtomicOp.GetOutputs(cxprogram)} {
				for _, argIdx := range args {
					index := arg
					arg++
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S0471.
					sig := cxprogram.GetCXTypeSignatureFromArray(argIdx)
					if !printerStackShown(sig, operator, k == 1) {
						continue
					}
					// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
					// Now an arena getter, site S0472.
					if _, err := cxprogram.GetPackageFromArray(sig.Package); err != nil {
						panic(err)
					}
					if printerStackShownBefore(cxprogram, op, sig, e, index) {
						continue
					}
					text = printerStackValueLine(text, cxprogram, fp, sig, cxAtomicOp.GetOperatorNameBytes(cxprogram))
					exprsShown = true
				}
			}
		}
		if exprsShown {
			text = append(text, '\n')
		} else {
			text = text[:exprsStart]
		}

		os.Stdout.Write(text)
		cxprogram.ProgramFormatBuffer = text[:0]
	}
}
