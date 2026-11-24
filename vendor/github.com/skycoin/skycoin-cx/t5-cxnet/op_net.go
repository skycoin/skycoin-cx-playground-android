package cxnet

import (
	"fmt"
	"net"
	"strconv"

	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-netbatch"
	"github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
)

// netUpdate is net.Update. A replay that runs out of updates ends the
// program, since the program then differs from the recorded one.
func netUpdate(waitMilliseconds int32) int32 {
	count := netbatch.NetUpdate(waitMilliseconds)
	if count < 0 {
		netReplayEndAssertFailure()
	}
	return count
}

// netReplayEndAssertFailure ends a program that asks for an update the
// replay record does not hold.
// VIOLATION STRHOT-328: E, the assert text of a replay past its record, built only when it fails, #508.
func netReplayEndAssertFailure() {
	assert.AssertPreconditionTrue(false, fmt.Sprintf("the net replay record has no update %d, or one of its entries names a socket the program did not open", netbatch.NetUpdateNumber()))
}

// netBufferView returns the bytes of the ui8 array value, in place in
// program memory.
func netBufferView(prgrm *ast.CXProgram, value *ast.CXValue) []byte {
	signature := value.TypeSignature
	if signature.Type != ast.TYPE_ARRAY_ATOMIC || signature.GetType(prgrm) != types.UI8 {
		netBufferAssertFailure()
	}
	return prgrm.Memory[value.Offset : value.Offset+signature.GetArraySize(prgrm)]
}

// netBufferAssertFailure ends a program that passes a buffer that is not a
// ui8 array.
// VIOLATION STRHOT-336: E, the assert text of a buffer that is not a ui8 array, built only when it fails, #508.
func netBufferAssertFailure() {
	assert.AssertPreconditionTrue(false, "a net buffer must be a ui8 array, such as var buffer [1024]ui8")
}

// netBufferLength clamps a length the program passes to the buffer.
func netBufferLength(buffer []byte, length int32) []byte {
	return buffer[:max(0, min(int(length), len(buffer)))]
}

func opNetUpdate(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, netUpdate(inputs[0].Get_i32(prgrm)))
}

func opNetReadyCount(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, netbatch.NetReadyCount())
}

func opNetReadyNext(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	handle, trigger, messageID, detail := netbatch.NetReadyNext()
	outputs[0].Set_i32(prgrm, handle)
	outputs[1].Set_i32(prgrm, int32(trigger))
	outputs[2].Set_i32(prgrm, messageID)
	outputs[3].Set_i32(prgrm, detail)
}

func opNetClock(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i64(prgrm, netbatch.NetClock())
}

func opNetRead(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	buffer := netBufferView(prgrm, &inputs[2])
	outputs[0].Set_i32(prgrm, netbatch.NetRead(inputs[0].Get_i32(prgrm), inputs[1].Get_i32(prgrm), buffer, inputs[3].Get_i32(prgrm)))
}

func opNetWrite(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	buffer := netBufferLength(netBufferView(prgrm, &inputs[1]), inputs[2].Get_i32(prgrm))
	outputs[0].Set_i32(prgrm, netbatch.NetWrite(inputs[0].Get_i32(prgrm), buffer))
}

func opNetClose(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, netbatch.NetClose(inputs[0].Get_i32(prgrm)))
}

func opNetTimeoutSet(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, netbatch.NetTimeoutSet(inputs[0].Get_i32(prgrm), inputs[1].Get_i32(prgrm)))
}

// VIOLATION STRHOT-329: B, hands an address to Go and the OS, #508.
func opNetTCPListen(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, netbatch.NetTCPListen(netAddress(inputs[0].Get_str(prgrm), inputs[1].Get_i32(prgrm))))
}

// VIOLATION STRHOT-330: B, hands an address to Go and the OS, #508.
func opNetTCPDial(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, netbatch.NetTCPDial(netAddress(inputs[0].Get_str(prgrm), inputs[1].Get_i32(prgrm))))
}

// VIOLATION STRHOT-331: B, hands an address to Go and the OS, #508.
func opNetUDPListen(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, netbatch.NetUDPListen(netAddress(inputs[0].Get_str(prgrm), inputs[1].Get_i32(prgrm))))
}

// VIOLATION STRHOT-332: B, hands an address to Go and the OS, #508.
func opNetUDPDial(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, netbatch.NetUDPDial(netAddress(inputs[0].Get_str(prgrm), inputs[1].Get_i32(prgrm))))
}

func opNetUDPReadFrom(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	buffer := netBufferView(prgrm, &inputs[2])
	length, peer := netbatch.NetUDPReadFrom(inputs[0].Get_i32(prgrm), inputs[1].Get_i32(prgrm), buffer, inputs[3].Get_i32(prgrm))
	outputs[0].Set_i32(prgrm, length)
	outputs[1].Set_i32(prgrm, peer)
}

func opNetUDPWriteTo(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	buffer := netBufferLength(netBufferView(prgrm, &inputs[2]), inputs[3].Get_i32(prgrm))
	outputs[0].Set_i32(prgrm, netbatch.NetUDPWriteTo(inputs[0].Get_i32(prgrm), inputs[1].Get_i32(prgrm), buffer))
}

// VIOLATION STRHOT-333: B, hands an address to Go, #508.
func opNetUDPPeerResolve(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, netbatch.NetUDPPeerResolve(netAddress(inputs[0].Get_str(prgrm), inputs[1].Get_i32(prgrm))))
}

// netBlock reads the inputs and writes the outputs of a vm form in order,
// as the lowering packs them.
type netBlock struct {
	blockMemory []byte
	blockOffset uint32
}

func (block *netBlock) blockI32() int32 {
	value := types.Read_i32(block.blockMemory, types.Pointer(block.blockOffset))
	block.blockOffset += 4
	return value
}

func (block *netBlock) blockI32Write(value int32) {
	types.Write_i32(block.blockMemory, types.Pointer(block.blockOffset), value)
	block.blockOffset += 4
}

func (block *netBlock) blockBoolWrite(value bool) {
	types.Write_bool(block.blockMemory, types.Pointer(block.blockOffset), value)
	block.blockOffset++
}

// blockBuffer returns the ui8 array at the address and size the lowering
// wrote, empty when it lies outside memory.
func (block *netBlock) blockBuffer() []byte {
	address := uint64(types.Read_ptr(block.blockMemory, types.Pointer(block.blockOffset)))
	size := uint64(types.Read_ptr(block.blockMemory, types.Pointer(block.blockOffset+netPointerSize)))
	block.blockOffset += 2 * netPointerSize
	if address+size > uint64(len(block.blockMemory)) {
		return nil
	}
	return block.blockMemory[address : address+size]
}

// blockStr returns the text of a str input as a Go string.
func (block *netBlock) blockStr() string {
	text, _ := vm.LibraryStrView(block.blockMemory, block.blockOffset)
	block.blockOffset += netPointerSize
	return string(text)
}

func libraryNetUpdateVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	block.blockI32Write(netUpdate(block.blockI32()))
}

func libraryNetReadyCountVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	block.blockI32Write(netbatch.NetReadyCount())
}

func libraryNetReadyNextVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	handle, trigger, messageID, detail := netbatch.NetReadyNext()
	block.blockI32Write(handle)
	block.blockI32Write(int32(trigger))
	block.blockI32Write(messageID)
	block.blockI32Write(detail)
}

func libraryNetClockVm(memory []byte, offset uint32) {
	types.Write_i64(memory, types.Pointer(offset), netbatch.NetClock())
}

func libraryNetReadVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	handle, messageID, buffer, payloadOffset := block.blockI32(), block.blockI32(), block.blockBuffer(), block.blockI32()
	block.blockI32Write(netbatch.NetRead(handle, messageID, buffer, payloadOffset))
}

func libraryNetWriteVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	handle, buffer := block.blockI32(), block.blockBuffer()
	buffer = netBufferLength(buffer, block.blockI32())
	block.blockI32Write(netbatch.NetWrite(handle, buffer))
}

func libraryNetCloseVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	block.blockBoolWrite(netbatch.NetClose(block.blockI32()))
}

func libraryNetTimeoutSetVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	handle, milliseconds := block.blockI32(), block.blockI32()
	block.blockBoolWrite(netbatch.NetTimeoutSet(handle, milliseconds))
}

func libraryNetTCPListenVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	host := block.blockStr()
	block.blockI32Write(netbatch.NetTCPListen(netAddress(host, block.blockI32())))
}

func libraryNetTCPDialVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	host := block.blockStr()
	block.blockI32Write(netbatch.NetTCPDial(netAddress(host, block.blockI32())))
}

func libraryNetUDPListenVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	host := block.blockStr()
	block.blockI32Write(netbatch.NetUDPListen(netAddress(host, block.blockI32())))
}

func libraryNetUDPDialVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	host := block.blockStr()
	block.blockI32Write(netbatch.NetUDPDial(netAddress(host, block.blockI32())))
}

func libraryNetUDPReadFromVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	handle, messageID, buffer, payloadOffset := block.blockI32(), block.blockI32(), block.blockBuffer(), block.blockI32()
	length, peer := netbatch.NetUDPReadFrom(handle, messageID, buffer, payloadOffset)
	block.blockI32Write(length)
	block.blockI32Write(peer)
}

func libraryNetUDPWriteToVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	handle, peer, buffer := block.blockI32(), block.blockI32(), block.blockBuffer()
	buffer = netBufferLength(buffer, block.blockI32())
	block.blockI32Write(netbatch.NetUDPWriteTo(handle, peer, buffer))
}

func libraryNetUDPPeerResolveVm(memory []byte, offset uint32) {
	block := netBlock{memory, offset}
	host := block.blockStr()
	block.blockI32Write(netbatch.NetUDPPeerResolve(netAddress(host, block.blockI32())))
}

// netAddress joins host and port as Go's net package takes them.
// VIOLATION STRHOT-337: B, builds the address it hands to Go and the OS, #508.
func netAddress(host string, port int32) string {
	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}
