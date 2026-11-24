package execute

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	// GCREACH: t3-ast owns the heap and the GC roots. The old loop runs opcodes that
	// allocate, which adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// RunCxAst runs cxprogram. A runtime error ends the run with nil and is left
// in cxprogram.RuntimeError for the printer package, see #413.
func RunCxAst(cxprogram *ast.CXProgram, untilEnd bool, maxOps int, untilCall types.Pointer) error {
	defer ast.RuntimeError(cxprogram)
	return runLoop(cxprogram, untilEnd, maxOps, untilCall, &cxprogram.ProgramInputValues, &cxprogram.ProgramOutputValues)
}

// runLoop is RunCxAst without the deferred RuntimeError. A panic goes up to
// the RunCxAst that runs the opcode which called Callback. The outer loop
// ticks the GC after each step. The loop Callback nests inside a native
// opcode does not, so nothing moves under that opcode.
// GCREACH: the GC tick, at a safe point where every root is in a slot,
// see docs/GarbageCollector.md.
func runLoop(cxprogram *ast.CXProgram, untilEnd bool, maxOps int, untilCall types.Pointer, inputs *[]ast.CXValue, outputs *[]ast.CXValue) error {
	var opCount int = 0
	gcTick := !untilCall.IsValid()
	for !cxprogram.Terminated && (untilEnd || (opCount < maxOps)) && (!untilCall.IsValid() || cxprogram.CallCounter > untilCall) {
		call := &cxprogram.CallStack[cxprogram.CallCounter]

		// checking if enough memory in stack
		if cxprogram.Stack.Pointer > constants.STACK_SIZE {
			panic(constants.STACK_OVERFLOW_ERROR)
		}

		opCount++

		err := call.Call(cxprogram, inputs, outputs)
		if err != nil {
			return err
		}
		if gcTick && ast.GarbageCollectorCheck(cxprogram) {
			ast.GarbageCollectorTrigger(cxprogram)
		}
	}

	return nil
}

// runSysInitFunc runs *init, ticking the GC after each step as runLoop does.
// A runtime error ends it with nil and is left in cxprogram.RuntimeError, as in RunCxAst.
// GCREACH: the GC tick of *init, see docs/GarbageCollector.md.
func runSysInitFunc(cxprogram *ast.CXProgram, mod *ast.CXPackage) error {
	defer ast.RuntimeError(cxprogram)
	fn, err := mod.SelectFunction(cxprogram, cxprogram.CxIrState.CxIrStateNameSysInit)
	if err != nil {
		return err
	}

	// *init function
	// OBJMODEL: state.CXCallMng.CXCallAlloc(), phase 4 #366.
	// Now a constructor call, site S1249.
	mainCall := MakeCall(cxprogram, fn)
	cxprogram.CallStack[0] = mainCall
	cxprogram.Stack.Pointer = fn.Size

	for !cxprogram.Terminated {
		call := &cxprogram.CallStack[cxprogram.CallCounter]
		err = call.Call(cxprogram, &cxprogram.ProgramInputValues, &cxprogram.ProgramOutputValues)
		if err != nil {
			return err
		}
		if ast.GarbageCollectorCheck(cxprogram) {
			ast.GarbageCollectorTrigger(cxprogram)
		}
	}
	// we reset call state
	cxprogram.Terminated = false
	cxprogram.CallCounter = 0
	cxprogram.CallStack[0].Operator = nil

	return nil
}

// GCREACH: allocates the os.Args strs and slice, which adds to
// the GC pressure, see docs/GarbageCollector.md.
func feedOSArgs(cxprogram *ast.CXProgram, args []string) error {
	// SelectPackage builds an error when os is not imported, so look first.
	if _, imported := cxprogram.Packages[cxprogram.CxIrState.CxIrStateNameOs]; !imported {
		return nil
	}
	if osPkg, err := cxprogram.SelectPackage(cxprogram.CxIrState.CxIrStateNameOs); err == nil {
		argsOffset := types.Pointer(0)
		if osGlbl, err := osPkg.GetGlobal(cxprogram, cxprogram.CxIrState.CxIrStateNameOsArgs); err == nil {
			for _, arg := range args {
				argOffset := ast.AllocateStr(cxprogram, arg)

				var argOffsetBytes [types.POINTER_SIZE]byte
				types.Write_ptr(argOffsetBytes[:], 0, argOffset)
				argsOffset = ast.WriteToSlice(cxprogram, argsOffset, argOffsetBytes[:])
			}
			types.Write_ptr(cxprogram.Memory, ast.GetFinalOffset(cxprogram, 0, nil, osGlbl), argsOffset)
		}
	}
	return nil
}

// RunPrepare sizes the heap and the buffers the old loop reuses, so a run of
// cxprogram allocates no Go memory for them (#407). RunCompiled calls it too,
// and it allocates nothing once cxprogram is prepared.
func RunPrepare(cxprogram *ast.CXProgram) {
	cxprogram.EnsureMinimumHeapSize()
	cxprogram.RunBuffersPrepare()
}

// RunCompiled runs cxprogram from *init and main, or goes on with the call
// in progress, for at most maxOps steps when maxOps is not 0.
func RunCompiled(cxprogram *ast.CXProgram, maxOps int, args []string) error {
	RunPrepare(cxprogram)

	var untilEnd bool
	if maxOps == 0 {
		untilEnd = true
	}

	// initializing program resources
	// cxprogram.Stacks = append(cxprogram.Stacks, MakeStack(1024))

	if cxprogram.CallStack[0].Operator == nil {
		mod, err := cxprogram.SelectPackage(cxprogram.CxIrState.CxIrStateNameMain)
		if err != nil {
			return err
		}

		// then the program is just starting and we need to run the SYS_INIT_FUNC
		err = runSysInitFunc(cxprogram, mod)
		if err != nil {
			return err
		}
		if cxprogram.RuntimeError.RuntimeErrorRecordRaised {
			return nil
		}

		fn, err := mod.SelectFunction(cxprogram, cxprogram.CxIrState.CxIrStateNameMain)
		if err != nil {
			return err
		}

		if len(fn.Expressions) < 1 {
			return nil
		}

		// main function
		// OBJMODEL: state.CXCallMng.CXCallAlloc(), phase 4 #366.
		// Now a constructor call, site S1250.
		mainCall := MakeCall(cxprogram, fn)
		mainCall.FramePointer = cxprogram.Stack.Pointer
		// initializing program resources
		cxprogram.CallStack[0] = mainCall

		// cxprogram.Stacks = append(cxprogram.Stacks, MakeStack(1024))
		cxprogram.Stack.Pointer += fn.Size

		// feeding os.Args
		err = feedOSArgs(cxprogram, args)
		if err != nil {
			return err
		}

		cxprogram.Terminated = false
	}

	if err := RunCxAst(cxprogram, untilEnd, maxOps, types.InvalidPointer); err != nil {
		return err
	}
	if cxprogram.RuntimeError.RuntimeErrorRecordRaised {
		return nil
	}

	if cxprogram.Terminated {
		cxprogram.Terminated = false
		cxprogram.CallCounter = 0
		cxprogram.CallStack[0].Operator = nil
	}

	// debugging memory
	// if len(cxprogram.Memory) < 2000 {
	// 	fmt.Println("cxprogram.Memory", cxprogram.Memory)
	// }

	return nil
}

// OBJMODEL: state.CXCallMng.CXCallAlloc(), phase 4 #366.
// Now a constructor, site S1251.
func MakeCall(prgrm *ast.CXProgram, op *ast.CXFunction) ast.CXCall {
	// OBJMODEL: state.CXCallMng.CXCallAlloc(), phase 4 #366.
	// Now a value literal, site S1252.
	return ast.CXCall{
		Operator:     op,
		Line:         0,
		FramePointer: 0,
		// Package:       pkg,
		// Program:       prgrm,
	}
}
