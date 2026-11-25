//go:build cxfx

package cxfx

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"golang.org/x/mobile/exp/audio/al"
)

func toBuffers(in interface{}) []al.Buffer { // REFACTOR : ??
	var out []al.Buffer
	var buffers []int32 = in.([]int32)
	for _, b := range buffers {
		out = append(out, al.Buffer(b))
	}
	return out
}

func toSources(in interface{}) []al.Source { // REFACTOR : ??
	var out []al.Source
	var sources []int32 = in.([]int32)
	for _, s := range sources {
		out = append(out, al.Source(s))
	}
	return out
}

func OpAlCloseDevice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	al.CloseDevice()
}

func OpAlDeleteBuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	buffers := toBuffers(inputs[0].GetSlice_i32(prgrm))
	al.DeleteBuffers(buffers...)
}

func OpAlDeleteSources(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	sources := toSources(inputs[0].GetSlice_i32(prgrm))
	al.DeleteSources(sources...)
}

func OpAlDeviceError(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	err := al.DeviceError()
	outputs[0].Set_i32(prgrm, err)
}

func OpAlError(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	err := al.Error()
	outputs[0].Set_i32(prgrm, err)
}

// VIOLATION STRHOT-192: B, OpenAL returns text, #432.
func OpAlExtensions(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	extensions := al.Extensions()
	outputs[0].Set_str(prgrm, extensions)
}

func OpAlOpenDevice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	if err := al.OpenDevice(); err != nil {
		panic(err)
	}
}

func OpAlPauseSources(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	sources := toSources(inputs[0].GetSlice_i32(prgrm))
	al.PauseSources(sources...)
}

func OpAlPlaySources(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	sources := toSources(inputs[0].GetSlice_i32(prgrm))
	al.PlaySources(sources...)
}

// VIOLATION STRHOT-193: B, OpenAL returns text, #432.
func OpAlRenderer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	renderer := al.Renderer()
	outputs[0].Set_str(prgrm, renderer)
}

func OpAlRewindSources(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	sources := toSources(inputs[0].GetSlice_i32(prgrm))
	al.RewindSources(sources...)
}

func OpAlStopSources(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	sources := toSources(inputs[0].GetSlice_i32(prgrm))
	al.StopSources(sources...)
}

// VIOLATION STRHOT-194: B, OpenAL returns text, #432.
func OpAlVendor(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	vendor := al.Vendor()
	outputs[0].Set_str(prgrm, vendor)
}

// VIOLATION STRHOT-195: B, OpenAL returns text, #432.
func OpAlVersion(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	version := al.Version()
	outputs[0].Set_str(prgrm, version)
}

func OpAlGenBuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	buffers := al.GenBuffers(int(inputs[0].Get_i32(prgrm)))
	outputSlicePointer := outputs[0].Offset
	outputSliceOffset := types.Read_ptr(prgrm.Memory, outputSlicePointer)
	for _, b := range buffers { // REFACTOR append with copy ?
		var obj [4]byte
		types.Write_i32(obj[:], 0, int32(b))
		outputSliceOffset = ast.WriteToSlice(prgrm, outputSliceOffset, obj[:])
	}
	outputs[0].Set_ptr(prgrm, outputSliceOffset)
}

func OpAlBufferData(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	buffer := al.Buffer(inputs[0].Get_i32(prgrm))
	format := inputs[1].Get_i32(prgrm)
	data := toBytes(inputs[2].GetSlice_ui8(prgrm))
	frequency := inputs[3].Get_i32(prgrm)
	buffer.BufferData(uint32(format), data, frequency)
}

func OpAlGenSources(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	sources := al.GenSources(int(inputs[0].Get_i32(prgrm)))
	outputSlicePointer := outputs[0].Offset
	outputSliceOffset := types.Read_ptr(prgrm.Memory, outputSlicePointer)
	for _, s := range sources { // REFACTOR append with copy ?
		var obj [4]byte
		types.Write_i32(obj[:], 0, int32(s))
		outputSliceOffset = ast.WriteToSlice(prgrm, outputSliceOffset, obj[:])
	}
	outputs[0].Set_ptr(prgrm, outputSliceOffset)
}

func OpAlSourceBuffersProcessed(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	source := al.Source(inputs[0].Get_i32(prgrm))
	outputs[0].Set_i32(prgrm, source.BuffersProcessed())
}

func OpAlSourceBuffersQueued(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	source := al.Source(inputs[0].Get_i32(prgrm))
	outputs[0].Set_i32(prgrm, source.BuffersQueued())
}

func OpAlSourceQueueBuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	source := al.Source(inputs[0].Get_i32(prgrm))
	buffers := toBuffers(inputs[1].GetSlice_i32(prgrm))
	source.QueueBuffers(buffers...)
}

func OpAlSourceState(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	source := al.Source(inputs[0].Get_i32(prgrm))
	outputs[0].Set_i32(prgrm, source.State())
}

func OpAlSourceUnqueueBuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	source := al.Source(inputs[0].Get_i32(prgrm))
	buffers := toBuffers(inputs[1].GetSlice_i32(prgrm))
	source.UnqueueBuffers(buffers...)
}
