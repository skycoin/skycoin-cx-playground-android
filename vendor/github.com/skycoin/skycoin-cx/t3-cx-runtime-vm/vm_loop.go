package vm

import (
	"bytes"
	"math"
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t2-cx-runtime-ir"
)

// loop runs instructions from the saved state until a stop, or stopSlow for
// an instruction the slow path runs.
func (machine *Machine) loop() Stop {
	mem := machine.MachineMemory
	pc := machine.machineResumePc
	fp := machine.MachineFramePointer
	sp := machine.MachineStackPointer
	code := machine.MachineImage.ImageFuncs[machine.MachineFunction].FuncCode
	for {
		if operationCountEnabled {
			OperationCount++
		}
		in := &code[pc]
		pc++
		mode := uint32(in.InstrMode)
		a := in.InstrA + fp&-(mode&1)
		b := in.InstrB + fp&-(mode>>1&1)
		c := in.InstrC + fp&-(mode>>2&1)
		switch in.InstrOpcode {
		case ir.OpcodeReturn:
			if machine.MachineDepth == 0 {
				return machine.stopAt(StopReturn, pc, fp, sp)
			}
			machine.MachineDepth--
			frame := &machine.MachineFrames[machine.MachineDepth]
			sp = fp
			pc, fp = frame.FramePc, frame.FramePointer
			machine.MachineFunction = frame.FrameFunction
			code = machine.MachineImage.ImageFuncs[frame.FrameFunction].FuncCode
		case ir.OpcodeJump:
			pc = in.InstrM
		case ir.OpcodeJumpIf:
			if mem[a] != 0 {
				pc = in.InstrM
			} else {
				pc = in.InstrN
			}
		case ir.OpcodeZero:
			memoryZero(mem, a, in.InstrN)
		case ir.OpcodeMove:
			// A scalar copy written out, as a call here would spill the loop state.
			switch destination, source := c, a; in.InstrN {
			case 4:
				wUI32(mem, destination, rUI32(mem, source))
			case 8:
				wUI64(mem, destination, rUI64(mem, source))
			case 1:
				mem[destination] = mem[source]
			case 2:
				wUI16(mem, destination, rUI16(mem, source))
			default:
				return machine.stopAt(stopSlow, pc, fp, sp)
			}
		case ir.OpcodeCallFrame:
			n := in.InstrN
			if machine.MachineDepth+1 >= ir.ImageCallDepthMax || sp+n > machine.MachineImage.ImageStackSize {
				return machine.stopAt(StopStackOverflow, pc, fp, sp)
			}
			memoryZero(mem, sp, n)
		case ir.OpcodeCallArg:
			// A scalar copy written out, as a call here would spill the loop state.
			switch destination, source := sp+in.InstrC, a; in.InstrN {
			case 4:
				wUI32(mem, destination, rUI32(mem, source))
			case 8:
				wUI64(mem, destination, rUI64(mem, source))
			case 1:
				mem[destination] = mem[source]
			case 2:
				wUI16(mem, destination, rUI16(mem, source))
			default:
				return machine.stopAt(stopSlow, pc, fp, sp)
			}
		case ir.OpcodeCall:
			// The OpcodeCallFrame before it checked the depth and the stack.
			machine.MachineFrames[machine.MachineDepth] = Frame{FrameFunction: machine.MachineFunction, FramePc: pc, FramePointer: fp}
			machine.MachineDepth++
			machine.MachineFunction = in.InstrM
			callee := &machine.MachineImage.ImageFuncs[in.InstrM]
			fp = sp
			sp += callee.FuncFrameSize
			code = callee.FuncCode
			pc = 0
		case ir.OpcodeCallResult:
			// A scalar copy written out, as a call here would spill the loop state.
			switch destination, source := c, sp+in.InstrA; in.InstrN {
			case 4:
				wUI32(mem, destination, rUI32(mem, source))
			case 8:
				wUI64(mem, destination, rUI64(mem, source))
			case 1:
				mem[destination] = mem[source]
			case 2:
				wUI16(mem, destination, rUI16(mem, source))
			default:
				return machine.stopAt(stopSlow, pc, fp, sp)
			}
		case ir.OpcodeAddressIndexStatic:
			index := rI32(mem, b)
			if index < 0 {
				return machine.stopAt(StopIndexNegative, pc, fp, sp)
			}
			wUI32(mem, c, a+uint32(index)*in.InstrN)
		case ir.OpcodeAddressIndex:
			index := rI32(mem, b)
			if index < 0 {
				return machine.stopAt(StopIndexNegative, pc, fp, sp)
			}
			wUI32(mem, c, rUI32(mem, a)+in.InstrM+uint32(index)*in.InstrN)
		case ir.OpcodeAddressLoad, ir.OpcodeAddressStore:
			// 4 bytes here, the other sizes and every error in the slow path.
			// More code here, or reading the opcode again, makes Go spill the
			// loop state.
			source, destination := a, c
			store := in.InstrD != 0
			if store {
				source = c
			}
			address := uint64(rUI32(mem, source)) + uint64(in.InstrM)
			if in.InstrN != 4 || address+4 > uint64(machine.machineDataEnd) {
				return machine.stopAt(stopSlow, pc, fp, sp)
			}
			if source = uint32(address); store {
				source, destination = a, uint32(address)
			}
			wUI32(mem, destination, rUI32(mem, source))
		case ir.OpcodeBoolNot:
			wBool(mem, c, !rBool(mem, a))
		case ir.OpcodeBoolAnd:
			wBool(mem, c, rBool(mem, a) && rBool(mem, b))
		case ir.OpcodeBoolOr:
			wBool(mem, c, rBool(mem, a) || rBool(mem, b))
		case ir.OpcodeBoolEq:
			wBool(mem, c, rBool(mem, a) == rBool(mem, b))
		case ir.OpcodeBoolUneq:
			wBool(mem, c, rBool(mem, a) != rBool(mem, b))
		case ir.OpcodeI8Add:
			wI8(mem, c, rI8(mem, a)+rI8(mem, b))
		case ir.OpcodeI8Sub:
			wI8(mem, c, rI8(mem, a)-rI8(mem, b))
		case ir.OpcodeI8Mul:
			wI8(mem, c, rI8(mem, a)*rI8(mem, b))
		case ir.OpcodeI8Div:
			d := rI8(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wI8(mem, c, rI8(mem, a)/d)
		case ir.OpcodeI8Mod:
			d := rI8(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wI8(mem, c, rI8(mem, a)%d)
		case ir.OpcodeI8Neg:
			wI8(mem, c, -rI8(mem, a))
		case ir.OpcodeI8Abs:
			v := rI8(mem, a)
			sign := v >> 7
			wI8(mem, c, (v^sign)-sign)
		case ir.OpcodeI8Max:
			v0, v1 := rI8(mem, a), rI8(mem, b)
			if v1 > v0 {
				v0 = v1
			}
			wI8(mem, c, v0)
		case ir.OpcodeI8Min:
			v0, v1 := rI8(mem, a), rI8(mem, b)
			if v1 < v0 {
				v0 = v1
			}
			wI8(mem, c, v0)
		case ir.OpcodeI8Eq:
			wBool(mem, c, rI8(mem, a) == rI8(mem, b))
		case ir.OpcodeI8Uneq:
			wBool(mem, c, rI8(mem, a) != rI8(mem, b))
		case ir.OpcodeI8Lt:
			wBool(mem, c, rI8(mem, a) < rI8(mem, b))
		case ir.OpcodeI8Lteq:
			wBool(mem, c, rI8(mem, a) <= rI8(mem, b))
		case ir.OpcodeI8Gt:
			wBool(mem, c, rI8(mem, a) > rI8(mem, b))
		case ir.OpcodeI8Gteq:
			wBool(mem, c, rI8(mem, a) >= rI8(mem, b))
		case ir.OpcodeI8BitAnd:
			wI8(mem, c, rI8(mem, a)&rI8(mem, b))
		case ir.OpcodeI8BitOr:
			wI8(mem, c, rI8(mem, a)|rI8(mem, b))
		case ir.OpcodeI8BitXor:
			wI8(mem, c, rI8(mem, a)^rI8(mem, b))
		case ir.OpcodeI8BitClear:
			wI8(mem, c, rI8(mem, a)&^rI8(mem, b))
		case ir.OpcodeI8BitShl:
			wI8(mem, c, rI8(mem, a)<<uint8(rI8(mem, b)))
		case ir.OpcodeI8BitShr:
			wI8(mem, c, rI8(mem, a)>>uint8(rI8(mem, b)))
		case ir.OpcodeI16Add:
			wI16(mem, c, rI16(mem, a)+rI16(mem, b))
		case ir.OpcodeI16Sub:
			wI16(mem, c, rI16(mem, a)-rI16(mem, b))
		case ir.OpcodeI16Mul:
			wI16(mem, c, rI16(mem, a)*rI16(mem, b))
		case ir.OpcodeI16Div:
			d := rI16(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wI16(mem, c, rI16(mem, a)/d)
		case ir.OpcodeI16Mod:
			d := rI16(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wI16(mem, c, rI16(mem, a)%d)
		case ir.OpcodeI16Neg:
			wI16(mem, c, -rI16(mem, a))
		case ir.OpcodeI16Abs:
			v := rI16(mem, a)
			sign := v >> 15
			wI16(mem, c, (v^sign)-sign)
		case ir.OpcodeI16Max:
			v0, v1 := rI16(mem, a), rI16(mem, b)
			if v1 > v0 {
				v0 = v1
			}
			wI16(mem, c, v0)
		case ir.OpcodeI16Min:
			v0, v1 := rI16(mem, a), rI16(mem, b)
			if v1 < v0 {
				v0 = v1
			}
			wI16(mem, c, v0)
		case ir.OpcodeI16Eq:
			wBool(mem, c, rI16(mem, a) == rI16(mem, b))
		case ir.OpcodeI16Uneq:
			wBool(mem, c, rI16(mem, a) != rI16(mem, b))
		case ir.OpcodeI16Lt:
			wBool(mem, c, rI16(mem, a) < rI16(mem, b))
		case ir.OpcodeI16Lteq:
			wBool(mem, c, rI16(mem, a) <= rI16(mem, b))
		case ir.OpcodeI16Gt:
			wBool(mem, c, rI16(mem, a) > rI16(mem, b))
		case ir.OpcodeI16Gteq:
			wBool(mem, c, rI16(mem, a) >= rI16(mem, b))
		case ir.OpcodeI16BitAnd:
			wI16(mem, c, rI16(mem, a)&rI16(mem, b))
		case ir.OpcodeI16BitOr:
			wI16(mem, c, rI16(mem, a)|rI16(mem, b))
		case ir.OpcodeI16BitXor:
			wI16(mem, c, rI16(mem, a)^rI16(mem, b))
		case ir.OpcodeI16BitClear:
			wI16(mem, c, rI16(mem, a)&^rI16(mem, b))
		case ir.OpcodeI16BitShl:
			wI16(mem, c, rI16(mem, a)<<uint16(rI16(mem, b)))
		case ir.OpcodeI16BitShr:
			wI16(mem, c, rI16(mem, a)>>uint16(rI16(mem, b)))
		case ir.OpcodeI32Add:
			wI32(mem, c, rI32(mem, a)+rI32(mem, b))
		case ir.OpcodeI32Sub:
			wI32(mem, c, rI32(mem, a)-rI32(mem, b))
		case ir.OpcodeI32Mul:
			wI32(mem, c, rI32(mem, a)*rI32(mem, b))
		case ir.OpcodeI32Div:
			d := rI32(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wI32(mem, c, rI32(mem, a)/d)
		case ir.OpcodeI32Mod:
			d := rI32(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wI32(mem, c, rI32(mem, a)%d)
		case ir.OpcodeI32Neg:
			wI32(mem, c, -rI32(mem, a))
		case ir.OpcodeI32Abs:
			v := rI32(mem, a)
			sign := v >> 31
			wI32(mem, c, (v^sign)-sign)
		case ir.OpcodeI32Max:
			v0, v1 := rI32(mem, a), rI32(mem, b)
			if v1 > v0 {
				v0 = v1
			}
			wI32(mem, c, v0)
		case ir.OpcodeI32Min:
			v0, v1 := rI32(mem, a), rI32(mem, b)
			if v1 < v0 {
				v0 = v1
			}
			wI32(mem, c, v0)
		case ir.OpcodeI32Eq:
			wBool(mem, c, rI32(mem, a) == rI32(mem, b))
		case ir.OpcodeI32Uneq:
			wBool(mem, c, rI32(mem, a) != rI32(mem, b))
		case ir.OpcodeI32Lt:
			wBool(mem, c, rI32(mem, a) < rI32(mem, b))
		case ir.OpcodeI32Lteq:
			wBool(mem, c, rI32(mem, a) <= rI32(mem, b))
		case ir.OpcodeI32Gt:
			wBool(mem, c, rI32(mem, a) > rI32(mem, b))
		case ir.OpcodeI32Gteq:
			wBool(mem, c, rI32(mem, a) >= rI32(mem, b))
		case ir.OpcodeI32BitAnd:
			wI32(mem, c, rI32(mem, a)&rI32(mem, b))
		case ir.OpcodeI32BitOr:
			wI32(mem, c, rI32(mem, a)|rI32(mem, b))
		case ir.OpcodeI32BitXor:
			wI32(mem, c, rI32(mem, a)^rI32(mem, b))
		case ir.OpcodeI32BitClear:
			wI32(mem, c, rI32(mem, a)&^rI32(mem, b))
		case ir.OpcodeI32BitShl:
			wI32(mem, c, rI32(mem, a)<<uint32(rI32(mem, b)))
		case ir.OpcodeI32BitShr:
			wI32(mem, c, rI32(mem, a)>>uint32(rI32(mem, b)))
		case ir.OpcodeI64Add:
			wI64(mem, c, rI64(mem, a)+rI64(mem, b))
		case ir.OpcodeI64Sub:
			wI64(mem, c, rI64(mem, a)-rI64(mem, b))
		case ir.OpcodeI64Mul:
			wI64(mem, c, rI64(mem, a)*rI64(mem, b))
		case ir.OpcodeI64Div:
			d := rI64(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wI64(mem, c, rI64(mem, a)/d)
		case ir.OpcodeI64Mod:
			d := rI64(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wI64(mem, c, rI64(mem, a)%d)
		case ir.OpcodeI64Neg:
			wI64(mem, c, -rI64(mem, a))
		case ir.OpcodeI64Abs:
			v := rI64(mem, a)
			sign := v >> 63
			wI64(mem, c, (v^sign)-sign)
		case ir.OpcodeI64Max:
			v0, v1 := rI64(mem, a), rI64(mem, b)
			if v1 > v0 {
				v0 = v1
			}
			wI64(mem, c, v0)
		case ir.OpcodeI64Min:
			v0, v1 := rI64(mem, a), rI64(mem, b)
			if v1 < v0 {
				v0 = v1
			}
			wI64(mem, c, v0)
		case ir.OpcodeI64Eq:
			wBool(mem, c, rI64(mem, a) == rI64(mem, b))
		case ir.OpcodeI64Uneq:
			wBool(mem, c, rI64(mem, a) != rI64(mem, b))
		case ir.OpcodeI64Lt:
			wBool(mem, c, rI64(mem, a) < rI64(mem, b))
		case ir.OpcodeI64Lteq:
			wBool(mem, c, rI64(mem, a) <= rI64(mem, b))
		case ir.OpcodeI64Gt:
			wBool(mem, c, rI64(mem, a) > rI64(mem, b))
		case ir.OpcodeI64Gteq:
			wBool(mem, c, rI64(mem, a) >= rI64(mem, b))
		case ir.OpcodeI64BitAnd:
			wI64(mem, c, rI64(mem, a)&rI64(mem, b))
		case ir.OpcodeI64BitOr:
			wI64(mem, c, rI64(mem, a)|rI64(mem, b))
		case ir.OpcodeI64BitXor:
			wI64(mem, c, rI64(mem, a)^rI64(mem, b))
		case ir.OpcodeI64BitClear:
			wI64(mem, c, rI64(mem, a)&^rI64(mem, b))
		case ir.OpcodeI64BitShl:
			wI64(mem, c, rI64(mem, a)<<uint64(rI64(mem, b)))
		case ir.OpcodeI64BitShr:
			wI64(mem, c, rI64(mem, a)>>uint64(rI64(mem, b)))
		case ir.OpcodeUI8Add:
			wUI8(mem, c, rUI8(mem, a)+rUI8(mem, b))
		case ir.OpcodeUI8Sub:
			wUI8(mem, c, rUI8(mem, a)-rUI8(mem, b))
		case ir.OpcodeUI8Mul:
			wUI8(mem, c, rUI8(mem, a)*rUI8(mem, b))
		case ir.OpcodeUI8Div:
			d := rUI8(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wUI8(mem, c, rUI8(mem, a)/d)
		case ir.OpcodeUI8Mod:
			d := rUI8(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wUI8(mem, c, rUI8(mem, a)%d)
		case ir.OpcodeUI8Max:
			v0, v1 := rUI8(mem, a), rUI8(mem, b)
			if v1 > v0 {
				v0 = v1
			}
			wUI8(mem, c, v0)
		case ir.OpcodeUI8Min:
			v0, v1 := rUI8(mem, a), rUI8(mem, b)
			if v1 < v0 {
				v0 = v1
			}
			wUI8(mem, c, v0)
		case ir.OpcodeUI8Eq:
			wBool(mem, c, rUI8(mem, a) == rUI8(mem, b))
		case ir.OpcodeUI8Uneq:
			wBool(mem, c, rUI8(mem, a) != rUI8(mem, b))
		case ir.OpcodeUI8Lt:
			wBool(mem, c, rUI8(mem, a) < rUI8(mem, b))
		case ir.OpcodeUI8Lteq:
			wBool(mem, c, rUI8(mem, a) <= rUI8(mem, b))
		case ir.OpcodeUI8Gt:
			wBool(mem, c, rUI8(mem, a) > rUI8(mem, b))
		case ir.OpcodeUI8Gteq:
			wBool(mem, c, rUI8(mem, a) >= rUI8(mem, b))
		case ir.OpcodeUI8BitAnd:
			wUI8(mem, c, rUI8(mem, a)&rUI8(mem, b))
		case ir.OpcodeUI8BitOr:
			wUI8(mem, c, rUI8(mem, a)|rUI8(mem, b))
		case ir.OpcodeUI8BitXor:
			wUI8(mem, c, rUI8(mem, a)^rUI8(mem, b))
		case ir.OpcodeUI8BitClear:
			wUI8(mem, c, rUI8(mem, a)&^rUI8(mem, b))
		case ir.OpcodeUI8BitShl:
			wUI8(mem, c, rUI8(mem, a)<<uint8(rUI8(mem, b)))
		case ir.OpcodeUI8BitShr:
			wUI8(mem, c, rUI8(mem, a)>>uint8(rUI8(mem, b)))
		case ir.OpcodeUI16Add:
			wUI16(mem, c, rUI16(mem, a)+rUI16(mem, b))
		case ir.OpcodeUI16Sub:
			wUI16(mem, c, rUI16(mem, a)-rUI16(mem, b))
		case ir.OpcodeUI16Mul:
			wUI16(mem, c, rUI16(mem, a)*rUI16(mem, b))
		case ir.OpcodeUI16Div:
			d := rUI16(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wUI16(mem, c, rUI16(mem, a)/d)
		case ir.OpcodeUI16Mod:
			d := rUI16(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wUI16(mem, c, rUI16(mem, a)%d)
		case ir.OpcodeUI16Max:
			v0, v1 := rUI16(mem, a), rUI16(mem, b)
			if v1 > v0 {
				v0 = v1
			}
			wUI16(mem, c, v0)
		case ir.OpcodeUI16Min:
			v0, v1 := rUI16(mem, a), rUI16(mem, b)
			if v1 < v0 {
				v0 = v1
			}
			wUI16(mem, c, v0)
		case ir.OpcodeUI16Eq:
			wBool(mem, c, rUI16(mem, a) == rUI16(mem, b))
		case ir.OpcodeUI16Uneq:
			wBool(mem, c, rUI16(mem, a) != rUI16(mem, b))
		case ir.OpcodeUI16Lt:
			wBool(mem, c, rUI16(mem, a) < rUI16(mem, b))
		case ir.OpcodeUI16Lteq:
			wBool(mem, c, rUI16(mem, a) <= rUI16(mem, b))
		case ir.OpcodeUI16Gt:
			wBool(mem, c, rUI16(mem, a) > rUI16(mem, b))
		case ir.OpcodeUI16Gteq:
			wBool(mem, c, rUI16(mem, a) >= rUI16(mem, b))
		case ir.OpcodeUI16BitAnd:
			wUI16(mem, c, rUI16(mem, a)&rUI16(mem, b))
		case ir.OpcodeUI16BitOr:
			wUI16(mem, c, rUI16(mem, a)|rUI16(mem, b))
		case ir.OpcodeUI16BitXor:
			wUI16(mem, c, rUI16(mem, a)^rUI16(mem, b))
		case ir.OpcodeUI16BitClear:
			wUI16(mem, c, rUI16(mem, a)&^rUI16(mem, b))
		case ir.OpcodeUI16BitShl:
			wUI16(mem, c, rUI16(mem, a)<<uint16(rUI16(mem, b)))
		case ir.OpcodeUI16BitShr:
			wUI16(mem, c, rUI16(mem, a)>>uint16(rUI16(mem, b)))
		case ir.OpcodeUI32Add:
			wUI32(mem, c, rUI32(mem, a)+rUI32(mem, b))
		case ir.OpcodeUI32Sub:
			wUI32(mem, c, rUI32(mem, a)-rUI32(mem, b))
		case ir.OpcodeUI32Mul:
			wUI32(mem, c, rUI32(mem, a)*rUI32(mem, b))
		case ir.OpcodeUI32Div:
			d := rUI32(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wUI32(mem, c, rUI32(mem, a)/d)
		case ir.OpcodeUI32Mod:
			d := rUI32(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wUI32(mem, c, rUI32(mem, a)%d)
		case ir.OpcodeUI32Max:
			v0, v1 := rUI32(mem, a), rUI32(mem, b)
			if v1 > v0 {
				v0 = v1
			}
			wUI32(mem, c, v0)
		case ir.OpcodeUI32Min:
			v0, v1 := rUI32(mem, a), rUI32(mem, b)
			if v1 < v0 {
				v0 = v1
			}
			wUI32(mem, c, v0)
		case ir.OpcodeUI32Eq:
			wBool(mem, c, rUI32(mem, a) == rUI32(mem, b))
		case ir.OpcodeUI32Uneq:
			wBool(mem, c, rUI32(mem, a) != rUI32(mem, b))
		case ir.OpcodeUI32Lt:
			wBool(mem, c, rUI32(mem, a) < rUI32(mem, b))
		case ir.OpcodeUI32Lteq:
			wBool(mem, c, rUI32(mem, a) <= rUI32(mem, b))
		case ir.OpcodeUI32Gt:
			wBool(mem, c, rUI32(mem, a) > rUI32(mem, b))
		case ir.OpcodeUI32Gteq:
			wBool(mem, c, rUI32(mem, a) >= rUI32(mem, b))
		case ir.OpcodeUI32BitAnd:
			wUI32(mem, c, rUI32(mem, a)&rUI32(mem, b))
		case ir.OpcodeUI32BitOr:
			wUI32(mem, c, rUI32(mem, a)|rUI32(mem, b))
		case ir.OpcodeUI32BitXor:
			wUI32(mem, c, rUI32(mem, a)^rUI32(mem, b))
		case ir.OpcodeUI32BitClear:
			wUI32(mem, c, rUI32(mem, a)&^rUI32(mem, b))
		case ir.OpcodeUI32BitShl:
			wUI32(mem, c, rUI32(mem, a)<<uint32(rUI32(mem, b)))
		case ir.OpcodeUI32BitShr:
			wUI32(mem, c, rUI32(mem, a)>>uint32(rUI32(mem, b)))
		case ir.OpcodeUI64Add:
			wUI64(mem, c, rUI64(mem, a)+rUI64(mem, b))
		case ir.OpcodeUI64Sub:
			wUI64(mem, c, rUI64(mem, a)-rUI64(mem, b))
		case ir.OpcodeUI64Mul:
			wUI64(mem, c, rUI64(mem, a)*rUI64(mem, b))
		case ir.OpcodeUI64Div:
			d := rUI64(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wUI64(mem, c, rUI64(mem, a)/d)
		case ir.OpcodeUI64Mod:
			d := rUI64(mem, b)
			if d == 0 {
				return machine.stopAt(StopDivideByZero, pc, fp, sp)
			}
			wUI64(mem, c, rUI64(mem, a)%d)
		case ir.OpcodeUI64Max:
			v0, v1 := rUI64(mem, a), rUI64(mem, b)
			if v1 > v0 {
				v0 = v1
			}
			wUI64(mem, c, v0)
		case ir.OpcodeUI64Min:
			v0, v1 := rUI64(mem, a), rUI64(mem, b)
			if v1 < v0 {
				v0 = v1
			}
			wUI64(mem, c, v0)
		case ir.OpcodeUI64Eq:
			wBool(mem, c, rUI64(mem, a) == rUI64(mem, b))
		case ir.OpcodeUI64Uneq:
			wBool(mem, c, rUI64(mem, a) != rUI64(mem, b))
		case ir.OpcodeUI64Lt:
			wBool(mem, c, rUI64(mem, a) < rUI64(mem, b))
		case ir.OpcodeUI64Lteq:
			wBool(mem, c, rUI64(mem, a) <= rUI64(mem, b))
		case ir.OpcodeUI64Gt:
			wBool(mem, c, rUI64(mem, a) > rUI64(mem, b))
		case ir.OpcodeUI64Gteq:
			wBool(mem, c, rUI64(mem, a) >= rUI64(mem, b))
		case ir.OpcodeUI64BitAnd:
			wUI64(mem, c, rUI64(mem, a)&rUI64(mem, b))
		case ir.OpcodeUI64BitOr:
			wUI64(mem, c, rUI64(mem, a)|rUI64(mem, b))
		case ir.OpcodeUI64BitXor:
			wUI64(mem, c, rUI64(mem, a)^rUI64(mem, b))
		case ir.OpcodeUI64BitClear:
			wUI64(mem, c, rUI64(mem, a)&^rUI64(mem, b))
		case ir.OpcodeUI64BitShl:
			wUI64(mem, c, rUI64(mem, a)<<uint64(rUI64(mem, b)))
		case ir.OpcodeUI64BitShr:
			wUI64(mem, c, rUI64(mem, a)>>uint64(rUI64(mem, b)))
		case ir.OpcodeF32Add:
			wF32(mem, c, rF32(mem, a)+rF32(mem, b))
		case ir.OpcodeF32Sub:
			wF32(mem, c, rF32(mem, a)-rF32(mem, b))
		case ir.OpcodeF32Mul:
			wF32(mem, c, rF32(mem, a)*rF32(mem, b))
		case ir.OpcodeF32Div:
			wF32(mem, c, rF32(mem, a)/rF32(mem, b))
		case ir.OpcodeF32Neg:
			wF32(mem, c, -rF32(mem, a))
		case ir.OpcodeF32Abs:
			wF32(mem, c, float32(math.Abs(float64(rF32(mem, a)))))
		case ir.OpcodeF32Eq:
			wBool(mem, c, rF32(mem, a) == rF32(mem, b))
		case ir.OpcodeF32Uneq:
			wBool(mem, c, rF32(mem, a) != rF32(mem, b))
		case ir.OpcodeF32Lt:
			wBool(mem, c, rF32(mem, a) < rF32(mem, b))
		case ir.OpcodeF32Lteq:
			wBool(mem, c, rF32(mem, a) <= rF32(mem, b))
		case ir.OpcodeF32Gt:
			wBool(mem, c, rF32(mem, a) > rF32(mem, b))
		case ir.OpcodeF32Gteq:
			wBool(mem, c, rF32(mem, a) >= rF32(mem, b))
		case ir.OpcodeF64Add:
			wF64(mem, c, rF64(mem, a)+rF64(mem, b))
		case ir.OpcodeF64Sub:
			wF64(mem, c, rF64(mem, a)-rF64(mem, b))
		case ir.OpcodeF64Mul:
			wF64(mem, c, rF64(mem, a)*rF64(mem, b))
		case ir.OpcodeF64Div:
			wF64(mem, c, rF64(mem, a)/rF64(mem, b))
		case ir.OpcodeF64Neg:
			wF64(mem, c, -rF64(mem, a))
		case ir.OpcodeF64Abs:
			wF64(mem, c, math.Abs(rF64(mem, a)))
		case ir.OpcodeF64Eq:
			wBool(mem, c, rF64(mem, a) == rF64(mem, b))
		case ir.OpcodeF64Uneq:
			wBool(mem, c, rF64(mem, a) != rF64(mem, b))
		case ir.OpcodeF64Lt:
			wBool(mem, c, rF64(mem, a) < rF64(mem, b))
		case ir.OpcodeF64Lteq:
			wBool(mem, c, rF64(mem, a) <= rF64(mem, b))
		case ir.OpcodeF64Gt:
			wBool(mem, c, rF64(mem, a) > rF64(mem, b))
		case ir.OpcodeF64Gteq:
			wBool(mem, c, rF64(mem, a) >= rF64(mem, b))
		case ir.OpcodeI8ToI16:
			wI16(mem, c, int16(rI8(mem, a)))
		case ir.OpcodeI8ToI32:
			wI32(mem, c, int32(rI8(mem, a)))
		case ir.OpcodeI8ToI64:
			wI64(mem, c, int64(rI8(mem, a)))
		case ir.OpcodeI8ToUI8:
			wUI8(mem, c, uint8(rI8(mem, a)))
		case ir.OpcodeI8ToUI16:
			wUI16(mem, c, uint16(rI8(mem, a)))
		case ir.OpcodeI8ToUI32:
			wUI32(mem, c, uint32(rI8(mem, a)))
		case ir.OpcodeI8ToUI64:
			wUI64(mem, c, uint64(rI8(mem, a)))
		case ir.OpcodeI8ToF32:
			wF32(mem, c, float32(rI8(mem, a)))
		case ir.OpcodeI8ToF64:
			wF64(mem, c, float64(rI8(mem, a)))
		case ir.OpcodeI16ToI8:
			wI8(mem, c, int8(rI16(mem, a)))
		case ir.OpcodeI16ToI32:
			wI32(mem, c, int32(rI16(mem, a)))
		case ir.OpcodeI16ToI64:
			wI64(mem, c, int64(rI16(mem, a)))
		case ir.OpcodeI16ToUI8:
			wUI8(mem, c, uint8(rI16(mem, a)))
		case ir.OpcodeI16ToUI16:
			wUI16(mem, c, uint16(rI16(mem, a)))
		case ir.OpcodeI16ToUI32:
			wUI32(mem, c, uint32(rI16(mem, a)))
		case ir.OpcodeI16ToUI64:
			wUI64(mem, c, uint64(rI16(mem, a)))
		case ir.OpcodeI16ToF32:
			wF32(mem, c, float32(rI16(mem, a)))
		case ir.OpcodeI16ToF64:
			wF64(mem, c, float64(rI16(mem, a)))
		case ir.OpcodeI32ToI8:
			wI8(mem, c, int8(rI32(mem, a)))
		case ir.OpcodeI32ToI16:
			wI16(mem, c, int16(rI32(mem, a)))
		case ir.OpcodeI32ToI64:
			wI64(mem, c, int64(rI32(mem, a)))
		case ir.OpcodeI32ToUI8:
			wUI8(mem, c, uint8(rI32(mem, a)))
		case ir.OpcodeI32ToUI16:
			wUI16(mem, c, uint16(rI32(mem, a)))
		case ir.OpcodeI32ToUI32:
			wUI32(mem, c, uint32(rI32(mem, a)))
		case ir.OpcodeI32ToUI64:
			wUI64(mem, c, uint64(rI32(mem, a)))
		case ir.OpcodeI32ToF32:
			wF32(mem, c, float32(rI32(mem, a)))
		case ir.OpcodeI32ToF64:
			wF64(mem, c, float64(rI32(mem, a)))
		case ir.OpcodeI64ToI8:
			wI8(mem, c, int8(rI64(mem, a)))
		case ir.OpcodeI64ToI16:
			wI16(mem, c, int16(rI64(mem, a)))
		case ir.OpcodeI64ToI32:
			wI32(mem, c, int32(rI64(mem, a)))
		case ir.OpcodeI64ToUI8:
			wUI8(mem, c, uint8(rI64(mem, a)))
		case ir.OpcodeI64ToUI16:
			wUI16(mem, c, uint16(rI64(mem, a)))
		case ir.OpcodeI64ToUI32:
			wUI32(mem, c, uint32(rI64(mem, a)))
		case ir.OpcodeI64ToUI64:
			wUI64(mem, c, uint64(rI64(mem, a)))
		case ir.OpcodeI64ToF32:
			wF32(mem, c, float32(rI64(mem, a)))
		case ir.OpcodeI64ToF64:
			wF64(mem, c, float64(rI64(mem, a)))
		case ir.OpcodeUI8ToI8:
			wI8(mem, c, int8(rUI8(mem, a)))
		case ir.OpcodeUI8ToI16:
			wI16(mem, c, int16(rUI8(mem, a)))
		case ir.OpcodeUI8ToI32:
			wI32(mem, c, int32(rUI8(mem, a)))
		case ir.OpcodeUI8ToI64:
			wI64(mem, c, int64(rUI8(mem, a)))
		case ir.OpcodeUI8ToUI16:
			wUI16(mem, c, uint16(rUI8(mem, a)))
		case ir.OpcodeUI8ToUI32:
			wUI32(mem, c, uint32(rUI8(mem, a)))
		case ir.OpcodeUI8ToUI64:
			wUI64(mem, c, uint64(rUI8(mem, a)))
		case ir.OpcodeUI8ToF32:
			wF32(mem, c, float32(rUI8(mem, a)))
		case ir.OpcodeUI8ToF64:
			wF64(mem, c, float64(rUI8(mem, a)))
		case ir.OpcodeUI16ToI8:
			wI8(mem, c, int8(rUI16(mem, a)))
		case ir.OpcodeUI16ToI16:
			wI16(mem, c, int16(rUI16(mem, a)))
		case ir.OpcodeUI16ToI32:
			wI32(mem, c, int32(rUI16(mem, a)))
		case ir.OpcodeUI16ToI64:
			wI64(mem, c, int64(rUI16(mem, a)))
		case ir.OpcodeUI16ToUI8:
			wUI8(mem, c, uint8(rUI16(mem, a)))
		case ir.OpcodeUI16ToUI32:
			wUI32(mem, c, uint32(rUI16(mem, a)))
		case ir.OpcodeUI16ToUI64:
			wUI64(mem, c, uint64(rUI16(mem, a)))
		case ir.OpcodeUI16ToF32:
			wF32(mem, c, float32(rUI16(mem, a)))
		case ir.OpcodeUI16ToF64:
			wF64(mem, c, float64(rUI16(mem, a)))
		case ir.OpcodeUI32ToI8:
			wI8(mem, c, int8(rUI32(mem, a)))
		case ir.OpcodeUI32ToI16:
			wI16(mem, c, int16(rUI32(mem, a)))
		case ir.OpcodeUI32ToI32:
			wI32(mem, c, int32(rUI32(mem, a)))
		case ir.OpcodeUI32ToI64:
			wI64(mem, c, int64(rUI32(mem, a)))
		case ir.OpcodeUI32ToUI8:
			wUI8(mem, c, uint8(rUI32(mem, a)))
		case ir.OpcodeUI32ToUI16:
			wUI16(mem, c, uint16(rUI32(mem, a)))
		case ir.OpcodeUI32ToUI64:
			wUI64(mem, c, uint64(rUI32(mem, a)))
		case ir.OpcodeUI32ToF32:
			wF32(mem, c, float32(rUI32(mem, a)))
		case ir.OpcodeUI32ToF64:
			wF64(mem, c, float64(rUI32(mem, a)))
		case ir.OpcodeUI64ToI8:
			wI8(mem, c, int8(rUI64(mem, a)))
		case ir.OpcodeUI64ToI16:
			wI16(mem, c, int16(rUI64(mem, a)))
		case ir.OpcodeUI64ToI32:
			wI32(mem, c, int32(rUI64(mem, a)))
		case ir.OpcodeUI64ToI64:
			wI64(mem, c, int64(rUI64(mem, a)))
		case ir.OpcodeUI64ToUI8:
			wUI8(mem, c, uint8(rUI64(mem, a)))
		case ir.OpcodeUI64ToUI16:
			wUI16(mem, c, uint16(rUI64(mem, a)))
		case ir.OpcodeUI64ToUI32:
			wUI32(mem, c, uint32(rUI64(mem, a)))
		case ir.OpcodeUI64ToF32:
			wF32(mem, c, float32(rUI64(mem, a)))
		case ir.OpcodeUI64ToF64:
			wF64(mem, c, float64(rUI64(mem, a)))
		case ir.OpcodeF32ToI8:
			wI8(mem, c, int8(rF32(mem, a)))
		case ir.OpcodeF32ToI16:
			wI16(mem, c, int16(rF32(mem, a)))
		case ir.OpcodeF32ToI32:
			wI32(mem, c, int32(rF32(mem, a)))
		case ir.OpcodeF32ToI64:
			wI64(mem, c, int64(rF32(mem, a)))
		case ir.OpcodeF32ToUI8:
			wUI8(mem, c, uint8(rF32(mem, a)))
		case ir.OpcodeF32ToUI16:
			wUI16(mem, c, uint16(rF32(mem, a)))
		case ir.OpcodeF32ToUI32:
			wUI32(mem, c, uint32(rF32(mem, a)))
		case ir.OpcodeF32ToUI64:
			wUI64(mem, c, uint64(rF32(mem, a)))
		case ir.OpcodeF32ToF64:
			wF64(mem, c, float64(rF32(mem, a)))
		case ir.OpcodeF64ToI8:
			wI8(mem, c, int8(rF64(mem, a)))
		case ir.OpcodeF64ToI16:
			wI16(mem, c, int16(rF64(mem, a)))
		case ir.OpcodeF64ToI32:
			wI32(mem, c, int32(rF64(mem, a)))
		case ir.OpcodeF64ToI64:
			wI64(mem, c, int64(rF64(mem, a)))
		case ir.OpcodeF64ToUI8:
			wUI8(mem, c, uint8(rF64(mem, a)))
		case ir.OpcodeF64ToUI16:
			wUI16(mem, c, uint16(rF64(mem, a)))
		case ir.OpcodeF64ToUI32:
			wUI32(mem, c, uint32(rF64(mem, a)))
		case ir.OpcodeF64ToUI64:
			wUI64(mem, c, uint64(rF64(mem, a)))
		case ir.OpcodeF64ToF32:
			wF32(mem, c, float32(rF64(mem, a)))
		default:
			return machine.stopAt(stopSlow, pc, fp, sp)
		}
	}
}

// slow runs the instruction at MachinePc that the loop handed over, the ones
// that call into Go: prints, test and float mod, max and min. It returns
// stopSlow to go on, or a stop.
func (machine *Machine) slow() Stop {
	if cap(machine.MachineOutput)-len(machine.MachineOutput) < machinePrintMax {
		return StopOutputFull
	}
	mem := machine.MachineMemory
	fp := machine.MachineFramePointer
	in := &machine.MachineImage.ImageFuncs[machine.MachineFunction].FuncCode[machine.MachinePc]
	mode := uint32(in.InstrMode)
	a := in.InstrA + fp&-(mode&1)
	b := in.InstrB + fp&-(mode>>1&1)
	c := in.InstrC + fp&-(mode>>2&1)
	machine.machineResumePc = machine.MachinePc + 1
	switch in.InstrOpcode {
	case ir.OpcodeMove:
		copy(mem[c:c+in.InstrN], mem[a:a+in.InstrN])
	case ir.OpcodeCallArg:
		sp := machine.MachineStackPointer
		copy(mem[sp+in.InstrC:sp+in.InstrC+in.InstrN], mem[a:a+in.InstrN])
	case ir.OpcodeCallResult:
		sp := machine.MachineStackPointer
		copy(mem[c:c+in.InstrN], mem[sp+in.InstrA:sp+in.InstrA+in.InstrN])
	case ir.OpcodeAddressLoad:
		source, valid := machine.address(rUI32(mem, a), in.InstrM, in.InstrN)
		if !valid {
			return machine.slowStop(StopAddressInvalid)
		}
		copy(mem[c:c+in.InstrN], mem[source:source+in.InstrN])
	case ir.OpcodeAddressStore:
		destination, valid := machine.address(rUI32(mem, c), in.InstrM, in.InstrN)
		if !valid {
			return machine.slowStop(StopAddressInvalid)
		}
		copy(mem[destination:destination+in.InstrN], mem[a:a+in.InstrN])
	case ir.OpcodeAddressDerefStatic:
		pointer := rPointer(mem, a)
		if pointer >= uint64(machine.machineDataEnd) {
			return machine.slowStop(StopAddressInvalid)
		}
		wUI32(mem, c, uint32(pointer))
	case ir.OpcodeAddressDeref:
		address, valid := machine.address(rUI32(mem, a), in.InstrM, machinePointerSize)
		if !valid {
			return machine.slowStop(StopAddressInvalid)
		}
		pointer := rPointer(mem, address)
		if pointer >= uint64(machine.machineDataEnd) {
			return machine.slowStop(StopAddressInvalid)
		}
		wUI32(mem, c, uint32(pointer))
	case ir.OpcodeAddressOf:
		wPointer(mem, c, a)
	case ir.OpcodeAddressPointer:
		wPointer(mem, c, rUI32(mem, a)+in.InstrM)
	case ir.OpcodePrintText:
		return machine.printBytes(machine.MachineImage.ImageText[in.InstrN:in.InstrN+in.InstrM], ir.PrintStrModeRaw)
	case ir.OpcodePrintStr:
		str, valid := memoryStrRead(mem, a, machine.machineDataEnd)
		if !valid {
			return machine.slowStop(StopAddressInvalid)
		}
		return machine.printBytes(str, ir.PrintStrMode(in.InstrN))
	case ir.OpcodeLibraryCall:
		if !machine.MachineImage.ImageLibraryEnabled[in.InstrM] {
			return machine.slowStop(StopLibraryDisabled)
		}
		// The lowering emits the call only for a vm form of a linked library.
		LibraryDispatchesVm[in.InstrM](in.InstrN, mem, machine.MachineStackPointer)
	case ir.OpcodePrintValue:
		machine.MachineOutput = printValue(machine.MachineOutput, mem, a, types.Code(in.InstrM), byte(in.InstrN))
	case ir.OpcodeTestEqual:
		if !bytes.Equal(mem[a:a+in.InstrN], mem[b:b+in.InstrM]) {
			machine.MachineTestFailures++
			return StopTestFailed
		}
	case ir.OpcodeBoolPrint:
		machine.MachineOutput = strconv.AppendBool(machine.MachineOutput, rBool(mem, a))
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeI8Print:
		machine.MachineOutput = strconv.AppendInt(machine.MachineOutput, int64(rI8(mem, a)), 10)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeI16Print:
		machine.MachineOutput = strconv.AppendInt(machine.MachineOutput, int64(rI16(mem, a)), 10)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeI32Print:
		machine.MachineOutput = strconv.AppendInt(machine.MachineOutput, int64(rI32(mem, a)), 10)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeI64Print:
		machine.MachineOutput = strconv.AppendInt(machine.MachineOutput, int64(rI64(mem, a)), 10)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeUI8Print:
		machine.MachineOutput = strconv.AppendUint(machine.MachineOutput, uint64(rUI8(mem, a)), 10)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeUI16Print:
		machine.MachineOutput = strconv.AppendUint(machine.MachineOutput, uint64(rUI16(mem, a)), 10)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeUI32Print:
		machine.MachineOutput = strconv.AppendUint(machine.MachineOutput, uint64(rUI32(mem, a)), 10)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeUI64Print:
		machine.MachineOutput = strconv.AppendUint(machine.MachineOutput, uint64(rUI64(mem, a)), 10)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeF32Mod:
		wF32(mem, c, float32(math.Mod(float64(rF32(mem, a)), float64(rF32(mem, b)))))
	case ir.OpcodeF32Max:
		wF32(mem, c, float32(math.Max(float64(rF32(mem, a)), float64(rF32(mem, b)))))
	case ir.OpcodeF32Min:
		wF32(mem, c, float32(math.Min(float64(rF32(mem, a)), float64(rF32(mem, b)))))
	case ir.OpcodeF32Print:
		machine.MachineOutput = strconv.AppendFloat(machine.MachineOutput, float64(rF32(mem, a)), 'g', -1, 32)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	case ir.OpcodeF64Mod:
		wF64(mem, c, math.Mod(rF64(mem, a), rF64(mem, b)))
	case ir.OpcodeF64Max:
		wF64(mem, c, math.Max(rF64(mem, a), rF64(mem, b)))
	case ir.OpcodeF64Min:
		wF64(mem, c, math.Min(rF64(mem, a), rF64(mem, b)))
	case ir.OpcodeF64Print:
		machine.MachineOutput = strconv.AppendFloat(machine.MachineOutput, float64(rF64(mem, a)), 'g', -1, 64)
		machine.MachineOutput = append(machine.MachineOutput, '\n')
	default:
		machine.machineResumePc = machine.MachinePc
		return StopOpcodeInvalid
	}
	return stopSlow
}

// printBytes writes src to the output, from machinePrintCursor on. When the
// output fills up first it keeps the cursor and stops with StopOutputFull at
// the same instruction, which runs again after the caller drains the output.
func (machine *Machine) printBytes(src []byte, mode ir.PrintStrMode) Stop {
	out := machine.MachineOutput
	cursor := int(machine.machinePrintCursor)
	for cursor < len(src) {
		if len(out) == cap(out) {
			return machine.printFull(out, cursor)
		}
		n := copy(out[len(out):cap(out)], src[cursor:])
		out = out[:len(out)+n]
		cursor += n
	}
	if mode == ir.PrintStrModeLine {
		if len(out) == cap(out) {
			return machine.printFull(out, cursor)
		}
		out = append(out, '\n')
	}
	machine.MachineOutput = out
	machine.machinePrintCursor = 0
	return stopSlow
}

// printFull saves a print that did not fit, to go on at cursor.
func (machine *Machine) printFull(out []byte, cursor int) Stop {
	machine.MachineOutput = out
	machine.machinePrintCursor = uint32(cursor)
	machine.machineResumePc = machine.MachinePc
	return StopOutputFull
}

// printValue appends the value of type typ at a in the printf verb, as
// buildString in t4-opcodes formats it. The lowering only emits the verb and
// type pairs handled here.
func printValue(out []byte, mem []byte, a uint32, typ types.Code, verb byte) []byte {
	switch typ {
	case types.BOOL:
		return strconv.AppendBool(out, rBool(mem, a))
	case types.I8:
		return strconv.AppendInt(out, int64(rI8(mem, a)), 10)
	case types.I16:
		return strconv.AppendInt(out, int64(rI16(mem, a)), 10)
	case types.I32:
		return strconv.AppendInt(out, int64(rI32(mem, a)), 10)
	case types.I64:
		return strconv.AppendInt(out, rI64(mem, a), 10)
	case types.UI8:
		return strconv.AppendUint(out, uint64(rUI8(mem, a)), 10)
	case types.UI16:
		return strconv.AppendUint(out, uint64(rUI16(mem, a)), 10)
	case types.UI32:
		return strconv.AppendUint(out, uint64(rUI32(mem, a)), 10)
	case types.UI64:
		return strconv.AppendUint(out, rUI64(mem, a), 10)
	case types.F32:
		if verb == 'f' {
			return strconv.AppendFloat(out, float64(rF32(mem, a)), 'f', 7, 32)
		}
		return strconv.AppendFloat(out, float64(rF32(mem, a)), 'g', -1, 32)
	case types.F64:
		if verb == 'f' {
			return strconv.AppendFloat(out, rF64(mem, a), 'f', 16, 64)
		}
		return strconv.AppendFloat(out, rF64(mem, a), 'g', -1, 64)
	}
	return out
}

// address returns base plus offset when size bytes from there are inside
// the stack and data segment.
func (machine *Machine) address(base uint32, offset uint32, size uint32) (uint32, bool) {
	address := uint64(base) + uint64(offset)
	if address+uint64(size) > uint64(machine.machineDataEnd) {
		return 0, false
	}
	return uint32(address), true
}

// slowStop ends a slow instruction with a stop at that instruction.
func (machine *Machine) slowStop(stop Stop) Stop {
	machine.machineResumePc = machine.MachinePc
	return stop
}
