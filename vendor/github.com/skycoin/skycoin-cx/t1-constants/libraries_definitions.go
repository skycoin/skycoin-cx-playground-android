package constants

// LibraryFunctionDefinitionTable gives every live function of
// LibraryFunctionLifecycleTable its handler, vm form and signature, by
// library and then number. t2-cx-core-opcodegen reads it and writes the
// signature tables of libraries_signatures_gen.go and one switch per
// library, so the library functions are fixed when cx is compiled (decision
// D96).
//
// Handler is the Go function the switch of the old loop calls,
// package.Function, with the package cxos (t5-cxos, the file and process
// libraries, left out by the nocxos tag), cxnet (t5-cxnet) or cipher
// (t5-cipher, left out by nocipher). HandlerVm is the vm form in the same
// package, "" for a function without one. Inputs and Outputs are spelled as
// in OpcodeDefinitionTable, and und is a byte buffer, a ui8 array of any
// length. Read only.
var LibraryFunctionDefinitionTable = [...]LibraryFunctionDefinition{
	// library, number, name, handler, vm handler, inputs, outputs
	{LibraryNumberFile, 1, "os.GetWorkingDirectory", "cxos.OpOsGetWorkingDirectory", "", "", "str"},
	{LibraryNumberFile, 2, "os.LogFile", "cxos.OpOsLogFile", "", "bool", ""},
	{LibraryNumberFile, 3, "os.Open", "cxos.OpOsOpen", "", "str", "i32"},
	{LibraryNumberFile, 4, "os.Create", "cxos.OpOsCreate", "", "str", "i32"},
	{LibraryNumberFile, 5, "os.Close", "cxos.OpOsClose", "cxos.LibraryFileCloseVm", "i32", "bool"},
	{LibraryNumberFile, 6, "os.Seek", "cxos.OpOsSeek", "cxos.LibraryFileSeekVm", "i32 i64 i32", "i64"},
	{LibraryNumberFile, 7, "os.ReadStr", "cxos.OpOsReadStr", "", "i32", "str bool"},
	{LibraryNumberFile, 8, "os.ReadF64", "cxos.OpOsReadF64", "", "i32", "f64 bool"},
	{LibraryNumberFile, 9, "os.ReadF32", "cxos.OpOsReadF32", "", "i32", "f32 bool"},
	{LibraryNumberFile, 10, "os.ReadUI64", "cxos.OpOsReadUI64", "", "i32", "ui64 bool"},
	{LibraryNumberFile, 11, "os.ReadUI32", "cxos.OpOsReadUI32", "", "i32", "ui32 bool"},
	{LibraryNumberFile, 12, "os.ReadUI16", "cxos.OpOsReadUI16", "", "i32", "ui16 bool"},
	{LibraryNumberFile, 13, "os.ReadUI8", "cxos.OpOsReadUI8", "", "i32", "ui8 bool"},
	{LibraryNumberFile, 14, "os.ReadI64", "cxos.OpOsReadI64", "", "i32", "i64 bool"},
	{LibraryNumberFile, 15, "os.ReadI32", "cxos.OpOsReadI32", "", "i32", "i32 bool"},
	{LibraryNumberFile, 16, "os.ReadI16", "cxos.OpOsReadI16", "", "i32", "i16 bool"},
	{LibraryNumberFile, 17, "os.ReadI8", "cxos.OpOsReadI8", "", "i32", "i8 bool"},
	{LibraryNumberFile, 18, "os.ReadBOOL", "cxos.OpOsReadBOOL", "", "i32", "bool bool"},
	{LibraryNumberFile, 19, "os.ReadF64Slice", "cxos.OpOsReadF64Slice", "", "i32 []f64 ui64", "[]f64 bool"},
	{LibraryNumberFile, 20, "os.ReadF32Slice", "cxos.OpOsReadF32Slice", "", "i32 []f32 ui64", "[]f32 bool"},
	{LibraryNumberFile, 21, "os.ReadUI64Slice", "cxos.OpOsReadUI64Slice", "", "i32 []ui64 ui64", "[]ui64 bool"},
	{LibraryNumberFile, 22, "os.ReadUI32Slice", "cxos.OpOsReadUI32Slice", "", "i32 []ui32 ui64", "[]ui32 bool"},
	{LibraryNumberFile, 23, "os.ReadUI16Slice", "cxos.OpOsReadUI16Slice", "", "i32 []ui16 ui64", "[]ui16 bool"},
	{LibraryNumberFile, 24, "os.ReadUI8Slice", "cxos.OpOsReadUI8Slice", "", "i32 []ui8 ui64", "[]ui8 bool"},
	{LibraryNumberFile, 25, "os.ReadI64Slice", "cxos.OpOsReadI64Slice", "", "i32 []i64 ui64", "[]i64 bool"},
	{LibraryNumberFile, 26, "os.ReadI32Slice", "cxos.OpOsReadI32Slice", "", "i32 []i32 ui64", "[]i32 bool"},
	{LibraryNumberFile, 27, "os.ReadI16Slice", "cxos.OpOsReadI16Slice", "", "i32 []i16 ui64", "[]i16 bool"},
	{LibraryNumberFile, 28, "os.ReadI8Slice", "cxos.OpOsReadI8Slice", "", "i32 []i8 ui64", "[]i8 bool"},
	{LibraryNumberFile, 29, "os.ReadAllText", "cxos.OpOsReadAllText", "", "str", "str bool"},
	{LibraryNumberFile, 30, "os.WriteStr", "cxos.OpOsWriteStr", "", "i32 str", "bool"},
	{LibraryNumberFile, 31, "os.WriteF64", "cxos.OpOsWriteF64", "", "i32 f64", "bool"},
	{LibraryNumberFile, 32, "os.WriteF32", "cxos.OpOsWriteF32", "", "i32 f32", "bool"},
	{LibraryNumberFile, 33, "os.WriteUI64", "cxos.OpOsWriteUI64", "", "i32 ui64", "bool"},
	{LibraryNumberFile, 34, "os.WriteUI32", "cxos.OpOsWriteUI32", "", "i32 ui32", "bool"},
	{LibraryNumberFile, 35, "os.WriteUI16", "cxos.OpOsWriteUI16", "", "i32 ui16", "bool"},
	{LibraryNumberFile, 36, "os.WriteUI8", "cxos.OpOsWriteUI8", "", "i32 ui8", "bool"},
	{LibraryNumberFile, 37, "os.WriteI64", "cxos.OpOsWriteI64", "", "i32 i64", "bool"},
	{LibraryNumberFile, 38, "os.WriteI32", "cxos.OpOsWriteI32", "", "i32 i32", "bool"},
	{LibraryNumberFile, 39, "os.WriteI16", "cxos.OpOsWriteI16", "", "i32 i16", "bool"},
	{LibraryNumberFile, 40, "os.WriteI8", "cxos.OpOsWriteI8", "", "i32 i8", "bool"},
	{LibraryNumberFile, 41, "os.WriteBOOL", "cxos.OpOsWriteBOOL", "", "i32 bool", "bool"},
	{LibraryNumberFile, 42, "os.WriteF64Slice", "cxos.OpOsWriteF64Slice", "", "i32 []f64", "bool"},
	{LibraryNumberFile, 43, "os.WriteF32Slice", "cxos.OpOsWriteF32Slice", "", "i32 []f32", "bool"},
	{LibraryNumberFile, 44, "os.WriteUI64Slice", "cxos.OpOsWriteUI64Slice", "", "i32 []ui64", "bool"},
	{LibraryNumberFile, 45, "os.WriteUI32Slice", "cxos.OpOsWriteUI32Slice", "", "i32 []ui32", "bool"},
	{LibraryNumberFile, 46, "os.WriteUI16Slice", "cxos.OpOsWriteUI16Slice", "", "i32 []ui16", "bool"},
	{LibraryNumberFile, 47, "os.WriteUI8Slice", "cxos.OpOsWriteUI8Slice", "", "i32 []ui8", "bool"},
	{LibraryNumberFile, 48, "os.WriteI64Slice", "cxos.OpOsWriteI64Slice", "", "i32 []i64", "bool"},
	{LibraryNumberFile, 49, "os.WriteI32Slice", "cxos.OpOsWriteI32Slice", "", "i32 []i32", "bool"},
	{LibraryNumberFile, 50, "os.WriteI16Slice", "cxos.OpOsWriteI16Slice", "", "i32 []i16", "bool"},
	{LibraryNumberFile, 51, "os.WriteI8Slice", "cxos.OpOsWriteI8Slice", "", "i32 []i8", "bool"},
	{LibraryNumberFile, 62, "StartCPUProfile", "cxos.OpStartProfile", "", "str i32", ""},
	{LibraryNumberFile, 63, "StopCPUProfile", "cxos.OpStopProfile", "", "str", ""},
	{LibraryNumberNet, 5, "net.Update", "cxnet.OpNetUpdate", "cxnet.LibraryNetUpdateVm", "i32", "i32"},
	{LibraryNumberNet, 6, "net.ReadyCount", "cxnet.OpNetReadyCount", "cxnet.LibraryNetReadyCountVm", "", "i32"},
	{LibraryNumberNet, 7, "net.ReadyNext", "cxnet.OpNetReadyNext", "cxnet.LibraryNetReadyNextVm", "", "i32 i32 i32 i32"},
	{LibraryNumberNet, 8, "net.Clock", "cxnet.OpNetClock", "cxnet.LibraryNetClockVm", "", "i64"},
	{LibraryNumberNet, 9, "net.Read", "cxnet.OpNetRead", "cxnet.LibraryNetReadVm", "i32 i32 und i32", "i32"},
	{LibraryNumberNet, 10, "net.Write", "cxnet.OpNetWrite", "cxnet.LibraryNetWriteVm", "i32 und i32", "i32"},
	{LibraryNumberNet, 11, "net.Close", "cxnet.OpNetClose", "cxnet.LibraryNetCloseVm", "i32", "bool"},
	{LibraryNumberNet, 12, "net.TimeoutSet", "cxnet.OpNetTimeoutSet", "cxnet.LibraryNetTimeoutSetVm", "i32 i32", "bool"},
	{LibraryNumberNet, 13, "net.TCPListen", "cxnet.OpNetTCPListen", "cxnet.LibraryNetTCPListenVm", "str i32", "i32"},
	{LibraryNumberNet, 14, "net.TCPDial", "cxnet.OpNetTCPDial", "cxnet.LibraryNetTCPDialVm", "str i32", "i32"},
	{LibraryNumberNet, 15, "net.UDPListen", "cxnet.OpNetUDPListen", "cxnet.LibraryNetUDPListenVm", "str i32", "i32"},
	{LibraryNumberNet, 16, "net.UDPDial", "cxnet.OpNetUDPDial", "cxnet.LibraryNetUDPDialVm", "str i32", "i32"},
	{LibraryNumberNet, 17, "net.UDPReadFrom", "cxnet.OpNetUDPReadFrom", "cxnet.LibraryNetUDPReadFromVm", "i32 i32 und i32", "i32 i32"},
	{LibraryNumberNet, 18, "net.UDPWriteTo", "cxnet.OpNetUDPWriteTo", "cxnet.LibraryNetUDPWriteToVm", "i32 i32 und i32", "i32"},
	{LibraryNumberNet, 19, "net.UDPPeerResolve", "cxnet.OpNetUDPPeerResolve", "cxnet.LibraryNetUDPPeerResolveVm", "str i32", "i32"},
	{LibraryNumberProcess, 1, "os.Run", "cxos.OpOsRun", "", "str i32 i32 str", "i32 i32 str"},
	{LibraryNumberCipher, 1, "cipher.HashOfBytes", "cipher.OpCipherHashOfBytes", "", "[]ui8", "[]ui8"},
	{LibraryNumberCipher, 2, "cipher.HashStateNew", "cipher.OpCipherHashStateNew", "cipher.LibraryCipherHashStateNewVm", "", "i32"},
	{LibraryNumberCipher, 3, "cipher.HashStateWrite", "cipher.OpCipherHashStateWrite", "", "i32 []ui8", ""},
	{LibraryNumberCipher, 4, "cipher.HashStateSum", "cipher.OpCipherHashStateSum", "", "i32", "[]ui8"},
	{LibraryNumberCipher, 5, "cipher.HashStateRelease", "cipher.OpCipherHashStateRelease", "cipher.LibraryCipherHashStateReleaseVm", "i32", ""},
	{LibraryNumberCipher, 6, "cipher.SignatureVerify", "cipher.OpCipherSignatureVerify", "", "[]ui8 []ui8 []ui8", "bool"},
	{LibraryNumberCipher, 7, "cipher.SignatureRecoverPubKey", "cipher.OpCipherSignatureRecoverPubKey", "", "[]ui8 []ui8", "[]ui8 bool"},
	{LibraryNumberCipher, 8, "cipher.SignatureSign", "cipher.OpCipherSignatureSign", "", "[]ui8 []ui8", "[]ui8 bool"},
	{LibraryNumberCipher, 9, "cipher.KeyPairGenerate", "cipher.OpCipherKeyPairGenerate", "", "", "[]ui8 []ui8"},
}

// LibraryFunctionDefinition is one row of LibraryFunctionDefinitionTable.
type LibraryFunctionDefinition struct {
	LibraryNumber int
	Number        int
	Name          string
	Handler       string
	HandlerVm     string
	Inputs        string
	Outputs       string
}
