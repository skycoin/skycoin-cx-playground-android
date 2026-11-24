package constants

// System libraries, see docs/Conventions.md and "System libraries" in
// LanguageGuide.md.
//
// A system library is a group of built-in functions that reach the operating
// system, such as files or the network, or that link a Go module outside the
// standard library, such as cipher. A program calls one through the
// single opcode OP_LIBRARY_CALL with a library number and a function number,
// not through a row of OpcodeLifecycleTable. A library runs only when it is
// enabled for the run, see LibraryPolicyEnabledWhenImported.
//
// Library numbers and the function numbers of each library follow the
// opcode rules. A number is fixed for life and stored in serialized programs.
// A new library or function takes the next number after the highest. A
// deleted function keeps its row as deprecated, gets no handler, and its
// number is never reused before the row's reassignable version, computed by
// LanguageVersionReassignable. Number 0 is never assigned.

// Library numbers. LibraryCount is one past the highest and sizes the per
// library tables.
const (
	LibraryNumberFile    = 1
	LibraryNumberNet     = 2
	LibraryNumberProcess = 3
	LibraryNumberCipher  = 4
	LibraryCount         = 5
)

// LibraryPolicyEnabledWhenImported is the default policy for a library that
// is not named in --enable. false keeps it disabled, so a program calls it
// only when the run enables it, `cx --enable=file,net program.cx`. true
// enables every library the program calls. Decision D17 on skycoin-cx#492.
const LibraryPolicyEnabledWhenImported = false

// Library is one row of LibraryTable.
type Library struct {
	LibraryNumber       int
	LibraryName         string
	LibraryVersionAdded int64
}

// LibraryTable is every system library, in number order. LibraryName is the
// name --enable takes. Read only.
var LibraryTable = [...]Library{
	{LibraryNumberFile, "file", 2611230001},
	{LibraryNumberNet, "net", 2611230001},
	{LibraryNumberProcess, "process", 2611230001},
	{LibraryNumberCipher, "cipher", 2611240003},
}

// LibraryByName returns the library named name.
func LibraryByName(name string) (Library, bool) {
	for _, library := range LibraryTable {
		if library.LibraryName == name {
			return library, true
		}
	}
	return Library{}, false
}

// LibraryNameOf returns the name of library number, "" when there is none.
func LibraryNameOf(number int) string {
	for _, library := range LibraryTable {
		if library.LibraryNumber == number {
			return library.LibraryName
		}
	}
	return ""
}

// LibraryFunctionLifecycle is one row of LibraryFunctionLifecycleTable.
type LibraryFunctionLifecycle struct {
	LibraryNumber       int
	Number              int
	Name                string
	VersionAdded        int64
	VersionDeprecated   int64
	VersionReassignable int64
}

// LibraryFunctionLifecycleTable is every function number ever assigned in a
// system library, by library and then number. The name is the CX name the
// program calls. The file, net and process functions were opcodes before
// 2611230001, and cipher.KeyPairGenerate was cipher.GenerateKeyPair before
// 2611240003, see the deprecated rows of OpcodeLifecycleTable. The blocking
// tcp functions 1 to 4 gave way to the batched net functions in 2611240004.
// Read only.
var LibraryFunctionLifecycleTable = [...]LibraryFunctionLifecycle{
	// library, number, name, version added, version deprecated, version reassignable
	{LibraryNumberFile, 1, "os.GetWorkingDirectory", 2611230001, 0, 0},
	{LibraryNumberFile, 2, "os.LogFile", 2611230001, 0, 0},
	{LibraryNumberFile, 3, "os.Open", 2611230001, 0, 0},
	{LibraryNumberFile, 4, "os.Create", 2611230001, 0, 0},
	{LibraryNumberFile, 5, "os.Close", 2611230001, 0, 0},
	{LibraryNumberFile, 6, "os.Seek", 2611230001, 0, 0},
	{LibraryNumberFile, 7, "os.ReadStr", 2611230001, 0, 0},
	{LibraryNumberFile, 8, "os.ReadF64", 2611230001, 0, 0},
	{LibraryNumberFile, 9, "os.ReadF32", 2611230001, 0, 0},
	{LibraryNumberFile, 10, "os.ReadUI64", 2611230001, 0, 0},
	{LibraryNumberFile, 11, "os.ReadUI32", 2611230001, 0, 0},
	{LibraryNumberFile, 12, "os.ReadUI16", 2611230001, 0, 0},
	{LibraryNumberFile, 13, "os.ReadUI8", 2611230001, 0, 0},
	{LibraryNumberFile, 14, "os.ReadI64", 2611230001, 0, 0},
	{LibraryNumberFile, 15, "os.ReadI32", 2611230001, 0, 0},
	{LibraryNumberFile, 16, "os.ReadI16", 2611230001, 0, 0},
	{LibraryNumberFile, 17, "os.ReadI8", 2611230001, 0, 0},
	{LibraryNumberFile, 18, "os.ReadBOOL", 2611230001, 0, 0},
	{LibraryNumberFile, 19, "os.ReadF64Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 20, "os.ReadF32Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 21, "os.ReadUI64Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 22, "os.ReadUI32Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 23, "os.ReadUI16Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 24, "os.ReadUI8Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 25, "os.ReadI64Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 26, "os.ReadI32Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 27, "os.ReadI16Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 28, "os.ReadI8Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 29, "os.ReadAllText", 2611230001, 0, 0},
	{LibraryNumberFile, 30, "os.WriteStr", 2611230001, 0, 0},
	{LibraryNumberFile, 31, "os.WriteF64", 2611230001, 0, 0},
	{LibraryNumberFile, 32, "os.WriteF32", 2611230001, 0, 0},
	{LibraryNumberFile, 33, "os.WriteUI64", 2611230001, 0, 0},
	{LibraryNumberFile, 34, "os.WriteUI32", 2611230001, 0, 0},
	{LibraryNumberFile, 35, "os.WriteUI16", 2611230001, 0, 0},
	{LibraryNumberFile, 36, "os.WriteUI8", 2611230001, 0, 0},
	{LibraryNumberFile, 37, "os.WriteI64", 2611230001, 0, 0},
	{LibraryNumberFile, 38, "os.WriteI32", 2611230001, 0, 0},
	{LibraryNumberFile, 39, "os.WriteI16", 2611230001, 0, 0},
	{LibraryNumberFile, 40, "os.WriteI8", 2611230001, 0, 0},
	{LibraryNumberFile, 41, "os.WriteBOOL", 2611230001, 0, 0},
	{LibraryNumberFile, 42, "os.WriteF64Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 43, "os.WriteF32Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 44, "os.WriteUI64Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 45, "os.WriteUI32Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 46, "os.WriteUI16Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 47, "os.WriteUI8Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 48, "os.WriteI64Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 49, "os.WriteI32Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 50, "os.WriteI16Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 51, "os.WriteI8Slice", 2611230001, 0, 0},
	{LibraryNumberFile, 52, "json.Open", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 53, "json.Close", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 54, "json.More", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 55, "json.Next", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 56, "json.Type", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 57, "json.Delim", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 58, "json.Bool", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 59, "json.Float64", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 60, "json.Int64", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 61, "json.Str", 2611230001, 2611240002, 2711240001},
	{LibraryNumberFile, 62, "StartCPUProfile", 2611230001, 0, 0},
	{LibraryNumberFile, 63, "StopCPUProfile", 2611230001, 0, 0},
	{LibraryNumberNet, 1, "tcp.Dial", 2611230001, 2611240004, 2711240001},
	{LibraryNumberNet, 2, "tcp.Listen", 2611230001, 2611240004, 2711240001},
	{LibraryNumberNet, 3, "tcp.Accept", 2611230001, 2611240004, 2711240001},
	{LibraryNumberNet, 4, "tcp.Close", 2611230001, 2611240004, 2711240001},
	{LibraryNumberNet, 5, "net.Update", 2611240004, 0, 0},
	{LibraryNumberNet, 6, "net.ReadyCount", 2611240004, 0, 0},
	{LibraryNumberNet, 7, "net.ReadyNext", 2611240004, 0, 0},
	{LibraryNumberNet, 8, "net.Clock", 2611240004, 0, 0},
	{LibraryNumberNet, 9, "net.Read", 2611240004, 0, 0},
	{LibraryNumberNet, 10, "net.Write", 2611240004, 0, 0},
	{LibraryNumberNet, 11, "net.Close", 2611240004, 0, 0},
	{LibraryNumberNet, 12, "net.TimeoutSet", 2611240004, 0, 0},
	{LibraryNumberNet, 13, "net.TCPListen", 2611240004, 0, 0},
	{LibraryNumberNet, 14, "net.TCPDial", 2611240004, 0, 0},
	{LibraryNumberNet, 15, "net.UDPListen", 2611240004, 0, 0},
	{LibraryNumberNet, 16, "net.UDPDial", 2611240004, 0, 0},
	{LibraryNumberNet, 17, "net.UDPReadFrom", 2611240004, 0, 0},
	{LibraryNumberNet, 18, "net.UDPWriteTo", 2611240004, 0, 0},
	{LibraryNumberNet, 19, "net.UDPPeerResolve", 2611240004, 0, 0},
	{LibraryNumberProcess, 1, "os.Run", 2611230001, 0, 0},
	{LibraryNumberCipher, 1, "cipher.HashOfBytes", 2611240003, 0, 0},
	{LibraryNumberCipher, 2, "cipher.HashStateNew", 2611240003, 0, 0},
	{LibraryNumberCipher, 3, "cipher.HashStateWrite", 2611240003, 0, 0},
	{LibraryNumberCipher, 4, "cipher.HashStateSum", 2611240003, 0, 0},
	{LibraryNumberCipher, 5, "cipher.HashStateRelease", 2611240003, 0, 0},
	{LibraryNumberCipher, 6, "cipher.SignatureVerify", 2611240003, 0, 0},
	{LibraryNumberCipher, 7, "cipher.SignatureRecoverPubKey", 2611240003, 0, 0},
	{LibraryNumberCipher, 8, "cipher.SignatureSign", 2611240003, 0, 0},
	{LibraryNumberCipher, 9, "cipher.KeyPairGenerate", 2611240003, 0, 0},
}

// LibraryFunctionLiveByName returns the live function named name of library.
func LibraryFunctionLiveByName(library int, name string) (LibraryFunctionLifecycle, bool) {
	for _, row := range LibraryFunctionLifecycleTable {
		if row.LibraryNumber == library && row.Name == name && row.VersionDeprecated == 0 {
			return row, true
		}
	}
	return LibraryFunctionLifecycle{}, false
}

// LibraryFunctionLiveByNumber returns the live function number of library.
func LibraryFunctionLiveByNumber(library int, number int) (LibraryFunctionLifecycle, bool) {
	for _, row := range LibraryFunctionLifecycleTable {
		if row.LibraryNumber == library && row.Number == number && row.VersionDeprecated == 0 {
			return row, true
		}
	}
	return LibraryFunctionLifecycle{}, false
}
