package constants

// Opcode numbering conventions, see docs/Conventions.md.
//
// Every opcode has a fixed number, listed below in number order. The number is
// stored in serialized programs, so it never depends on registration order or
// build tags, and an opcode keeps it for life. A new opcode takes the next
// number after the highest in the table.
//
// A deleted opcode is deprecated, not removed. Its row stays with the version
// it was deprecated in, nothing registers a handler on its number, and the gap
// it leaves is never filled before its reassignable version. Reassigning a
// number adds a new row with the same number after the deprecated one.
//
// A renamed opcode keeps its number and its row, which takes the new name.
// OpcodeRenameTable records the old name and the version of the rename, the
// old name can no longer be called, and it is never given to another opcode.
//
// Columns are number, name, version added, version deprecated and version
// reassignable. A live opcode has 0 in the last two. An opcode older than
// language versioning has version added 0.
//
// A version is the integer yymmddnnnn, the release date and a release number
// on that day, for example 2611200001 is 2026-11-20 release 0001. Reassignable
// is the deprecated version plus OpcodeReassignmentIntervalYears years, same
// month and day, release 0001, so 2611200001 becomes 2711200001. A 29 February
// deprecation becomes reassignable on 1 March. LanguageVersionReassignable
// computes it, and opcodes_history.go lists every lifecycle event in order.
//
// docs/opcodes.csv holds the same table, with the renames of each row in a
// former_names column as old@version separated by semicolons.
// TestOpcodeLifecycleTableMatchesCSV fails when they differ, and
// CX_UPDATE_OPCODES_CSV=1 rewrites the CSV.
//
// Numbers 19 to 38, 45 to 72 and 85 to 126 sit inside the operator range
// markers below and were never assigned.

// OpcodeLifecycleTable is every opcode number ever assigned. Read only.
var OpcodeLifecycleTable = [...]OpcodeLifecycle{
	// number, name, version added, version deprecated, version reassignable
	{1, "identity", 0, 0, 0},
	{2, "jmp", 0, 0, 0},
	{3, "absjmp", 0, 0, 0},
	{4, "jmpeq", 0, 0, 0},
	{5, "jmpuneq", 0, 0, 0},
	{6, "jmpgt", 0, 0, 0},
	{7, "jmpgteq", 0, 0, 0},
	{8, "jmplt", 0, 0, 0},
	{9, "jmplteq", 0, 0, 0},
	{10, "jmpzero", 0, 0, 0},
	{11, "jmpnotzero", 0, 0, 0},
	{12, "nop", 0, 0, 0},
	{13, "goto", 0, 0, 0},
	{14, "break", 0, 0, 0},
	{15, "continue", 0, 0, 0},
	{16, "debug", 0, 0, 0},
	{17, "serialize", 0, 0, 0},
	{18, "deserialize", 0, 0, 0},
	{39, "eq", 0, 0, 0},
	{40, "uneq", 0, 0, 0},
	{41, "lt", 0, 0, 0},
	{42, "gt", 0, 0, 0},
	{43, "lteq", 0, 0, 0},
	{44, "gteq", 0, 0, 0},
	{73, "bitand", 0, 0, 0},
	{74, "bitor", 0, 0, 0},
	{75, "bitxor", 0, 0, 0},
	{76, "bitclear", 0, 0, 0},
	{77, "bitshl", 0, 0, 0},
	{78, "bitshr", 0, 0, 0},
	{79, "mul", 0, 0, 0},
	{80, "div", 0, 0, 0},
	{81, "mod", 0, 0, 0},
	{82, "add", 0, 0, 0},
	{83, "sub", 0, 0, 0},
	{84, "neg", 0, 0, 0},
	{127, "assert", 0, 0, 0},
	{128, "test", 0, 0, 0},
	{129, "panic", 0, 0, 0},
	{130, "append", 0, 0, 0},
	{131, "bool.or", 0, 0, 0},
	{132, "bool.and", 0, 0, 0},
	{133, "bool.not", 0, 0, 0},
	{134, "cipher.GenerateKeyPair", 0, 2611240003, 2711240001},
	{135, "time.Sleep", 0, 0, 0},
	{136, "time.UnixMilli", 0, 0, 0},
	{137, "time.UnixNano", 0, 0, 0},
	{138, "os.GetWorkingDirectory", 0, 2611230001, 2711230001},
	{139, "os.LogFile", 0, 2611230001, 2711230001},
	{140, "os.Open", 0, 2611230001, 2711230001},
	{141, "os.Create", 0, 2611230001, 2711230001},
	{142, "os.Close", 0, 2611230001, 2711230001},
	{143, "os.Seek", 0, 2611230001, 2711230001},
	{144, "os.ReadStr", 0, 2611230001, 2711230001},
	{145, "os.ReadF64", 0, 2611230001, 2711230001},
	{146, "os.ReadF32", 0, 2611230001, 2711230001},
	{147, "os.ReadUI64", 0, 2611230001, 2711230001},
	{148, "os.ReadUI32", 0, 2611230001, 2711230001},
	{149, "os.ReadUI16", 0, 2611230001, 2711230001},
	{150, "os.ReadUI8", 0, 2611230001, 2711230001},
	{151, "os.ReadI64", 0, 2611230001, 2711230001},
	{152, "os.ReadI32", 0, 2611230001, 2711230001},
	{153, "os.ReadI16", 0, 2611230001, 2711230001},
	{154, "os.ReadI8", 0, 2611230001, 2711230001},
	{155, "os.ReadBOOL", 0, 2611230001, 2711230001},
	{156, "os.ReadF64Slice", 0, 2611230001, 2711230001},
	{157, "os.ReadF32Slice", 0, 2611230001, 2711230001},
	{158, "os.ReadUI64Slice", 0, 2611230001, 2711230001},
	{159, "os.ReadUI32Slice", 0, 2611230001, 2711230001},
	{160, "os.ReadUI16Slice", 0, 2611230001, 2711230001},
	{161, "os.ReadUI8Slice", 0, 2611230001, 2711230001},
	{162, "os.ReadI64Slice", 0, 2611230001, 2711230001},
	{163, "os.ReadI32Slice", 0, 2611230001, 2711230001},
	{164, "os.ReadI16Slice", 0, 2611230001, 2711230001},
	{165, "os.ReadI8Slice", 0, 2611230001, 2711230001},
	{166, "os.ReadAllText", 0, 2611230001, 2711230001},
	{167, "os.WriteStr", 0, 2611230001, 2711230001},
	{168, "os.WriteF64", 0, 2611230001, 2711230001},
	{169, "os.WriteF32", 0, 2611230001, 2711230001},
	{170, "os.WriteUI64", 0, 2611230001, 2711230001},
	{171, "os.WriteUI32", 0, 2611230001, 2711230001},
	{172, "os.WriteUI16", 0, 2611230001, 2711230001},
	{173, "os.WriteUI8", 0, 2611230001, 2711230001},
	{174, "os.WriteI64", 0, 2611230001, 2711230001},
	{175, "os.WriteI32", 0, 2611230001, 2711230001},
	{176, "os.WriteI16", 0, 2611230001, 2711230001},
	{177, "os.WriteI8", 0, 2611230001, 2711230001},
	{178, "os.WriteBOOL", 0, 2611230001, 2711230001},
	{179, "os.WriteF64Slice", 0, 2611230001, 2711230001},
	{180, "os.WriteF32Slice", 0, 2611230001, 2711230001},
	{181, "os.WriteUI64Slice", 0, 2611230001, 2711230001},
	{182, "os.WriteUI32Slice", 0, 2611230001, 2711230001},
	{183, "os.WriteUI16Slice", 0, 2611230001, 2711230001},
	{184, "os.WriteUI8Slice", 0, 2611230001, 2711230001},
	{185, "os.WriteI64Slice", 0, 2611230001, 2711230001},
	{186, "os.WriteI32Slice", 0, 2611230001, 2711230001},
	{187, "os.WriteI16Slice", 0, 2611230001, 2711230001},
	{188, "os.WriteI8Slice", 0, 2611230001, 2711230001},
	{189, "os.Run", 0, 2611230001, 2711230001},
	{190, "os.Exit", 0, 0, 0},
	{191, "json.Open", 0, 2611230001, 2711230001},
	{192, "json.Close", 0, 2611230001, 2711230001},
	{193, "json.More", 0, 2611230001, 2711230001},
	{194, "json.Next", 0, 2611230001, 2711230001},
	{195, "json.Type", 0, 2611230001, 2711230001},
	{196, "json.Delim", 0, 2611230001, 2711230001},
	{197, "json.Bool", 0, 2611230001, 2711230001},
	{198, "json.Float64", 0, 2611230001, 2711230001},
	{199, "json.Int64", 0, 2611230001, 2711230001},
	{200, "json.Str", 0, 2611230001, 2711230001},
	{201, "StartCPUProfile", 0, 2611230001, 2711230001},
	{202, "StopCPUProfile", 0, 2611230001, 2711230001},
	{203, "regexp.Compile", 0, 0, 0},
	{204, "regexp.MustCompile", 0, 0, 0},
	{205, "regexp.Regexp.Find", 0, 0, 0},
	{206, "tcp.Dial", 0, 2611230001, 2711230001},
	{207, "tcp.Listen", 0, 2611230001, 2711230001},
	{208, "tcp.Accept", 0, 2611230001, 2711230001},
	{209, "tcp.Close", 0, 2611230001, 2711230001},
	{210, "resize", 0, 0, 0},
	{211, "insert", 0, 0, 0},
	{212, "remove", 0, 0, 0},
	{213, "copy", 0, 0, 0},
	{214, "len", 0, 0, 0},
	{215, "printf", 0, 0, 0},
	{216, "sprintf", 0, 0, 0},
	{217, "read", 0, 0, 0},
	{218, "bool.eq", 0, 0, 0},
	{219, "bool.uneq", 0, 0, 0},
	{220, "bool.print", 0, 0, 0},
	{221, "i8.str", 0, 0, 0},
	{222, "i8.i16", 0, 0, 0},
	{223, "i8.i32", 0, 0, 0},
	{224, "i8.i64", 0, 0, 0},
	{225, "i8.ui8", 0, 0, 0},
	{226, "i8.ui16", 0, 0, 0},
	{227, "i8.ui32", 0, 0, 0},
	{228, "i8.ui64", 0, 0, 0},
	{229, "i8.f32", 0, 0, 0},
	{230, "i8.f64", 0, 0, 0},
	{231, "i16.str", 0, 0, 0},
	{232, "i16.i8", 0, 0, 0},
	{233, "i16.i32", 0, 0, 0},
	{234, "i16.i64", 0, 0, 0},
	{235, "i16.ui8", 0, 0, 0},
	{236, "i16.ui16", 0, 0, 0},
	{237, "i16.ui32", 0, 0, 0},
	{238, "i16.ui64", 0, 0, 0},
	{239, "i16.f32", 0, 0, 0},
	{240, "i16.f64", 0, 0, 0},
	{241, "i32.str", 0, 0, 0},
	{242, "i32.i8", 0, 0, 0},
	{243, "i32.i16", 0, 0, 0},
	{244, "i32.i64", 0, 0, 0},
	{245, "i32.ui8", 0, 0, 0},
	{246, "i32.ui16", 0, 0, 0},
	{247, "i32.ui32", 0, 0, 0},
	{248, "i32.ui64", 0, 0, 0},
	{249, "i32.f32", 0, 0, 0},
	{250, "i32.f64", 0, 0, 0},
	{251, "i64.str", 0, 0, 0},
	{252, "i64.i8", 0, 0, 0},
	{253, "i64.i16", 0, 0, 0},
	{254, "i64.i32", 0, 0, 0},
	{255, "i64.ui8", 0, 0, 0},
	{256, "i64.ui16", 0, 0, 0},
	{257, "i64.ui32", 0, 0, 0},
	{258, "i64.ui64", 0, 0, 0},
	{259, "i64.f32", 0, 0, 0},
	{260, "i64.f64", 0, 0, 0},
	{261, "ui8.str", 0, 0, 0},
	{262, "ui8.i8", 0, 0, 0},
	{263, "ui8.i16", 0, 0, 0},
	{264, "ui8.i32", 0, 0, 0},
	{265, "ui8.i64", 0, 0, 0},
	{266, "ui8.ui16", 0, 0, 0},
	{267, "ui8.ui32", 0, 0, 0},
	{268, "ui8.ui64", 0, 0, 0},
	{269, "ui8.f32", 0, 0, 0},
	{270, "ui8.f64", 0, 0, 0},
	{271, "ui16.str", 0, 0, 0},
	{272, "ui16.i8", 0, 0, 0},
	{273, "ui16.i16", 0, 0, 0},
	{274, "ui16.i32", 0, 0, 0},
	{275, "ui16.i64", 0, 0, 0},
	{276, "ui16.ui8", 0, 0, 0},
	{277, "ui16.ui32", 0, 0, 0},
	{278, "ui16.ui64", 0, 0, 0},
	{279, "ui16.f32", 0, 0, 0},
	{280, "ui16.f64", 0, 0, 0},
	{281, "ui32.str", 0, 0, 0},
	{282, "ui32.i8", 0, 0, 0},
	{283, "ui32.i16", 0, 0, 0},
	{284, "ui32.i32", 0, 0, 0},
	{285, "ui32.i64", 0, 0, 0},
	{286, "ui32.ui8", 0, 0, 0},
	{287, "ui32.ui16", 0, 0, 0},
	{288, "ui32.ui64", 0, 0, 0},
	{289, "ui32.f32", 0, 0, 0},
	{290, "ui32.f64", 0, 0, 0},
	{291, "ui64.str", 0, 0, 0},
	{292, "ui64.i8", 0, 0, 0},
	{293, "ui64.i16", 0, 0, 0},
	{294, "ui64.i32", 0, 0, 0},
	{295, "ui64.i64", 0, 0, 0},
	{296, "ui64.ui8", 0, 0, 0},
	{297, "ui64.ui16", 0, 0, 0},
	{298, "ui64.ui32", 0, 0, 0},
	{299, "ui64.f32", 0, 0, 0},
	{300, "ui64.f64", 0, 0, 0},
	{301, "f32.str", 0, 0, 0},
	{302, "f32.i8", 0, 0, 0},
	{303, "f32.i16", 0, 0, 0},
	{304, "f32.i32", 0, 0, 0},
	{305, "f32.i64", 0, 0, 0},
	{306, "f32.ui8", 0, 0, 0},
	{307, "f32.ui16", 0, 0, 0},
	{308, "f32.ui32", 0, 0, 0},
	{309, "f32.ui64", 0, 0, 0},
	{310, "f32.f64", 0, 0, 0},
	{311, "f64.str", 0, 0, 0},
	{312, "f64.i8", 0, 0, 0},
	{313, "f64.i16", 0, 0, 0},
	{314, "f64.i32", 0, 0, 0},
	{315, "f64.i64", 0, 0, 0},
	{316, "f64.ui8", 0, 0, 0},
	{317, "f64.ui16", 0, 0, 0},
	{318, "f64.ui32", 0, 0, 0},
	{319, "f64.ui64", 0, 0, 0},
	{320, "f64.f32", 0, 0, 0},
	{321, "i8.eq", 0, 0, 0},
	{322, "i8.uneq", 0, 0, 0},
	{323, "i8.bitand", 0, 0, 0},
	{324, "i8.bitor", 0, 0, 0},
	{325, "i8.bitxor", 0, 0, 0},
	{326, "i8.bitclear", 0, 0, 0},
	{327, "i8.bitshl", 0, 0, 0},
	{328, "i8.bitshr", 0, 0, 0},
	{329, "i8.add", 0, 0, 0},
	{330, "i8.sub", 0, 0, 0},
	{331, "i8.neg", 0, 0, 0},
	{332, "i8.mul", 0, 0, 0},
	{333, "i8.div", 0, 0, 0},
	{334, "i8.mod", 0, 0, 0},
	{335, "i8.gt", 0, 0, 0},
	{336, "i8.gteq", 0, 0, 0},
	{337, "i8.lt", 0, 0, 0},
	{338, "i8.lteq", 0, 0, 0},
	{339, "i8.print", 0, 0, 0},
	{340, "i8.abs", 0, 0, 0},
	{341, "i8.max", 0, 0, 0},
	{342, "i8.min", 0, 0, 0},
	{343, "i8.rand", 0, 0, 0},
	{344, "i16.eq", 0, 0, 0},
	{345, "i16.uneq", 0, 0, 0},
	{346, "i16.bitand", 0, 0, 0},
	{347, "i16.bitor", 0, 0, 0},
	{348, "i16.bitxor", 0, 0, 0},
	{349, "i16.bitclear", 0, 0, 0},
	{350, "i16.bitshl", 0, 0, 0},
	{351, "i16.bitshr", 0, 0, 0},
	{352, "i16.add", 0, 0, 0},
	{353, "i16.sub", 0, 0, 0},
	{354, "i16.neg", 0, 0, 0},
	{355, "i16.mul", 0, 0, 0},
	{356, "i16.div", 0, 0, 0},
	{357, "i16.mod", 0, 0, 0},
	{358, "i16.gt", 0, 0, 0},
	{359, "i16.gteq", 0, 0, 0},
	{360, "i16.lt", 0, 0, 0},
	{361, "i16.lteq", 0, 0, 0},
	{362, "i16.print", 0, 0, 0},
	{363, "i16.abs", 0, 0, 0},
	{364, "i16.max", 0, 0, 0},
	{365, "i16.min", 0, 0, 0},
	{366, "i16.rand", 0, 0, 0},
	{367, "i32.jmpeq", 0, 0, 0},
	{368, "i32.jmpuneq", 0, 0, 0},
	{369, "i32.jmpgt", 0, 0, 0},
	{370, "i32.jmpgteq", 0, 0, 0},
	{371, "i32.jmplt", 0, 0, 0},
	{372, "i32.jmplteq", 0, 0, 0},
	{373, "i32.jmpzero", 0, 0, 0},
	{374, "i32.jmpnotzero", 0, 0, 0},
	{375, "i32.eq", 0, 0, 0},
	{376, "i32.uneq", 0, 0, 0},
	{377, "i32.bitand", 0, 0, 0},
	{378, "i32.bitor", 0, 0, 0},
	{379, "i32.bitxor", 0, 0, 0},
	{380, "i32.bitclear", 0, 0, 0},
	{381, "i32.bitshl", 0, 0, 0},
	{382, "i32.bitshr", 0, 0, 0},
	{383, "i32.add", 0, 0, 0},
	{384, "i32.sub", 0, 0, 0},
	{385, "i32.neg", 0, 0, 0},
	{386, "i32.mul", 0, 0, 0},
	{387, "i32.div", 0, 0, 0},
	{388, "i32.mod", 0, 0, 0},
	{389, "i32.gt", 0, 0, 0},
	{390, "i32.gteq", 0, 0, 0},
	{391, "i32.lt", 0, 0, 0},
	{392, "i32.lteq", 0, 0, 0},
	{393, "i32.print", 0, 0, 0},
	{394, "i32.abs", 0, 0, 0},
	{395, "i32.max", 0, 0, 0},
	{396, "i32.min", 0, 0, 0},
	{397, "i32.rand", 0, 0, 0},
	{398, "i64.eq", 0, 0, 0},
	{399, "i64.uneq", 0, 0, 0},
	{400, "i64.bitand", 0, 0, 0},
	{401, "i64.bitor", 0, 0, 0},
	{402, "i64.bitxor", 0, 0, 0},
	{403, "i64.bitclear", 0, 0, 0},
	{404, "i64.bitshl", 0, 0, 0},
	{405, "i64.bitshr", 0, 0, 0},
	{406, "i64.add", 0, 0, 0},
	{407, "i64.sub", 0, 0, 0},
	{408, "i64.neg", 0, 0, 0},
	{409, "i64.mul", 0, 0, 0},
	{410, "i64.div", 0, 0, 0},
	{411, "i64.mod", 0, 0, 0},
	{412, "i64.gt", 0, 0, 0},
	{413, "i64.gteq", 0, 0, 0},
	{414, "i64.lt", 0, 0, 0},
	{415, "i64.lteq", 0, 0, 0},
	{416, "i64.print", 0, 0, 0},
	{417, "i64.abs", 0, 0, 0},
	{418, "i64.max", 0, 0, 0},
	{419, "i64.min", 0, 0, 0},
	{420, "i64.rand", 0, 0, 0},
	{421, "ui8.eq", 0, 0, 0},
	{422, "ui8.uneq", 0, 0, 0},
	{423, "ui8.bitand", 0, 0, 0},
	{424, "ui8.bitor", 0, 0, 0},
	{425, "ui8.bitxor", 0, 0, 0},
	{426, "ui8.bitclear", 0, 0, 0},
	{427, "ui8.bitshl", 0, 0, 0},
	{428, "ui8.bitshr", 0, 0, 0},
	{429, "ui8.add", 0, 0, 0},
	{430, "ui8.sub", 0, 0, 0},
	{431, "ui8.mul", 0, 0, 0},
	{432, "ui8.div", 0, 0, 0},
	{433, "ui8.mod", 0, 0, 0},
	{434, "ui8.gt", 0, 0, 0},
	{435, "ui8.gteq", 0, 0, 0},
	{436, "ui8.lt", 0, 0, 0},
	{437, "ui8.lteq", 0, 0, 0},
	{438, "ui8.print", 0, 0, 0},
	{439, "ui8.max", 0, 0, 0},
	{440, "ui8.min", 0, 0, 0},
	{441, "ui8.rand", 0, 0, 0},
	{442, "ui16.eq", 0, 0, 0},
	{443, "ui16.uneq", 0, 0, 0},
	{444, "ui16.bitand", 0, 0, 0},
	{445, "ui16.bitor", 0, 0, 0},
	{446, "ui16.bitxor", 0, 0, 0},
	{447, "ui16.bitclear", 0, 0, 0},
	{448, "ui16.bitshl", 0, 0, 0},
	{449, "ui16.bitshr", 0, 0, 0},
	{450, "ui16.add", 0, 0, 0},
	{451, "ui16.sub", 0, 0, 0},
	{452, "ui16.mul", 0, 0, 0},
	{453, "ui16.div", 0, 0, 0},
	{454, "ui16.mod", 0, 0, 0},
	{455, "ui16.gt", 0, 0, 0},
	{456, "ui16.gteq", 0, 0, 0},
	{457, "ui16.lt", 0, 0, 0},
	{458, "ui16.lteq", 0, 0, 0},
	{459, "ui16.print", 0, 0, 0},
	{460, "ui16.max", 0, 0, 0},
	{461, "ui16.min", 0, 0, 0},
	{462, "ui16.rand", 0, 0, 0},
	{463, "ui32.eq", 0, 0, 0},
	{464, "ui32.uneq", 0, 0, 0},
	{465, "ui32.bitand", 0, 0, 0},
	{466, "ui32.bitor", 0, 0, 0},
	{467, "ui32.bitxor", 0, 0, 0},
	{468, "ui32.bitclear", 0, 0, 0},
	{469, "ui32.bitshl", 0, 0, 0},
	{470, "ui32.bitshr", 0, 0, 0},
	{471, "ui32.add", 0, 0, 0},
	{472, "ui32.sub", 0, 0, 0},
	{473, "ui32.mul", 0, 0, 0},
	{474, "ui32.div", 0, 0, 0},
	{475, "ui32.mod", 0, 0, 0},
	{476, "ui32.gt", 0, 0, 0},
	{477, "ui32.gteq", 0, 0, 0},
	{478, "ui32.lt", 0, 0, 0},
	{479, "ui32.lteq", 0, 0, 0},
	{480, "ui32.print", 0, 0, 0},
	{481, "ui32.max", 0, 0, 0},
	{482, "ui32.min", 0, 0, 0},
	{483, "ui32.rand", 0, 0, 0},
	{484, "ui64.eq", 0, 0, 0},
	{485, "ui64.uneq", 0, 0, 0},
	{486, "ui64.bitand", 0, 0, 0},
	{487, "ui64.bitor", 0, 0, 0},
	{488, "ui64.bitxor", 0, 0, 0},
	{489, "ui64.bitclear", 0, 0, 0},
	{490, "ui64.bitshl", 0, 0, 0},
	{491, "ui64.bitshr", 0, 0, 0},
	{492, "ui64.add", 0, 0, 0},
	{493, "ui64.sub", 0, 0, 0},
	{494, "ui64.mul", 0, 0, 0},
	{495, "ui64.div", 0, 0, 0},
	{496, "ui64.mod", 0, 0, 0},
	{497, "ui64.gt", 0, 0, 0},
	{498, "ui64.gteq", 0, 0, 0},
	{499, "ui64.lt", 0, 0, 0},
	{500, "ui64.lteq", 0, 0, 0},
	{501, "ui64.print", 0, 0, 0},
	{502, "ui64.max", 0, 0, 0},
	{503, "ui64.min", 0, 0, 0},
	{504, "ui64.rand", 0, 0, 0},
	{505, "f32.eq", 0, 0, 0},
	{506, "f32.uneq", 0, 0, 0},
	{507, "f32.add", 0, 0, 0},
	{508, "f32.sub", 0, 0, 0},
	{509, "f32.neg", 0, 0, 0},
	{510, "f32.mul", 0, 0, 0},
	{511, "f32.div", 0, 0, 0},
	{512, "f32.mod", 0, 0, 0},
	{513, "f32.gt", 0, 0, 0},
	{514, "f32.gteq", 0, 0, 0},
	{515, "f32.lt", 0, 0, 0},
	{516, "f32.lteq", 0, 0, 0},
	{517, "f32.isnan", 0, 0, 0},
	{518, "f32.print", 0, 0, 0},
	{519, "f32.abs", 0, 0, 0},
	{520, "f32.pow", 0, 0, 0},
	{521, "f32.acos", 0, 0, 0},
	{522, "f32.cos", 0, 0, 0},
	{523, "f32.asin", 0, 0, 0},
	{524, "f32.sin", 0, 0, 0},
	{525, "f32.sqrt", 0, 0, 0},
	{526, "f32.log", 0, 0, 0},
	{527, "f32.log2", 0, 0, 0},
	{528, "f32.log10", 0, 0, 0},
	{529, "f32.max", 0, 0, 0},
	{530, "f32.min", 0, 0, 0},
	{531, "f32.rand", 0, 0, 0},
	{532, "f64.eq", 0, 0, 0},
	{533, "f64.uneq", 0, 0, 0},
	{534, "f64.add", 0, 0, 0},
	{535, "f64.sub", 0, 0, 0},
	{536, "f64.neg", 0, 0, 0},
	{537, "f64.mul", 0, 0, 0},
	{538, "f64.div", 0, 0, 0},
	{539, "f64.mod", 0, 0, 0},
	{540, "f64.gt", 0, 0, 0},
	{541, "f64.gteq", 0, 0, 0},
	{542, "f64.lt", 0, 0, 0},
	{543, "f64.lteq", 0, 0, 0},
	{544, "f64.isnan", 0, 0, 0},
	{545, "f64.print", 0, 0, 0},
	{546, "f64.abs", 0, 0, 0},
	{547, "f64.pow", 0, 0, 0},
	{548, "f64.acos", 0, 0, 0},
	{549, "f64.cos", 0, 0, 0},
	{550, "f64.asin", 0, 0, 0},
	{551, "f64.sin", 0, 0, 0},
	{552, "f64.sqrt", 0, 0, 0},
	{553, "f64.log", 0, 0, 0},
	{554, "f64.log2", 0, 0, 0},
	{555, "f64.log10", 0, 0, 0},
	{556, "f64.max", 0, 0, 0},
	{557, "f64.min", 0, 0, 0},
	{558, "f64.rand", 0, 0, 0},
	{559, "str.eq", 0, 2611200001, 2711200001},   // 559 str.eq: added 0000000000, deprecated 2611200001, reassignable from 2711200001
	{560, "str.uneq", 0, 2611200001, 2711200001}, // 560 str.uneq: added 0000000000, deprecated 2611200001, reassignable from 2711200001
	{561, "string.Concatenation", 0, 0, 0},
	{562, "string.ParseDecimalI8", 0, 0, 0},
	{563, "string.ParseDecimalI16", 0, 0, 0},
	{564, "string.ParseDecimalI32", 0, 0, 0},
	{565, "string.ParseDecimalI64", 0, 0, 0},
	{566, "string.ParseDecimalUI8", 0, 0, 0},
	{567, "string.ParseDecimalUI16", 0, 0, 0},
	{568, "string.ParseDecimalUI32", 0, 0, 0},
	{569, "string.ParseDecimalUI64", 0, 0, 0},
	{570, "string.ParseFloatF32", 0, 0, 0},
	{571, "string.ParseFloatF64", 0, 0, 0},
	{572, "string.PrintLine", 0, 0, 0},
	{573, "string.SubstringByByteRange", 0, 0, 0},
	{574, "string.ByteIndexOfFirstSubstring", 0, 0, 0},
	{575, "string.ByteIndexOfLastSubstring", 0, 0, 0},
	{576, "string.TrimSurroundingWhitespace", 0, 0, 0},
	{577, "panicIf", 0, 0, 0},
	{578, "panicIfNot", 0, 0, 0},
	{579, "strerror", 0, 0, 0},
	{580, "aff.print", 0, 0, 0},
	{581, "aff.query", 0, 0, 0},
	{582, "str.lt", 0, 2611200001, 2711200001},   // 582 str.lt: added 0000000000, deprecated 2611200001, reassignable from 2711200001
	{583, "str.lteq", 0, 2611200001, 2711200001}, // 583 str.lteq: added 0000000000, deprecated 2611200001, reassignable from 2711200001
	{584, "str.gt", 0, 2611200001, 2711200001},   // 584 str.gt: added 0000000000, deprecated 2611200001, reassignable from 2711200001
	{585, "str.gteq", 0, 2611200001, 2711200001}, // 585 str.gteq: added 0000000000, deprecated 2611200001, reassignable from 2711200001
	{586, "string.EqualityByByteValue", 2611200001, 0, 0},
	{587, "string.EqualityByHeapPointer", 2611200001, 0, 0},
	{588, "string.OrderingByByteValue", 2611200001, 0, 0},
	{589, "ptr.eq", 2611200001, 0, 0},
	{590, "ptr.uneq", 2611200001, 0, 0},
	{591, "gl.Init", 0, 0, 0},
	{592, "gl.Destroy", 0, 2611240001, 2711240001},
	{593, "gl.Strs", 0, 2611240001, 2711240001},
	{594, "gl.Free", 0, 2611240001, 2711240001},
	{595, "gl.NewTexture", 0, 0, 0},
	{596, "gl.NewTextureCube", 0, 0, 0},
	{597, "gl.NewGIF", 0, 0, 0},
	{598, "gl.FreeGIF", 0, 0, 0},
	{599, "gl.GIFFrameToTexture", 0, 0, 0},
	{600, "gl.UploadImageToTexture", 0, 0, 0},
	{601, "gl.CxReleaseTexture", 0, 0, 0},
	{602, "gl.CxTextureGetPixel", 0, 0, 0},
	{603, "gl.AppendF32", 0, 0, 0},
	{604, "gl.AppendUI16", 0, 0, 0},
	{605, "gl.AppendUI32", 0, 0, 0},
	{606, "gl.MatrixMode", 0, 0, 0},
	{607, "gl.Rotatef", 0, 0, 0},
	{608, "gl.Translatef", 0, 0, 0},
	{609, "gl.LoadIdentity", 0, 0, 0},
	{610, "gl.PushMatrix", 0, 0, 0},
	{611, "gl.PopMatrix", 0, 0, 0},
	{612, "gl.EnableClientState", 0, 0, 0},
	{613, "gl.Color3f", 0, 0, 0},
	{614, "gl.Color4f", 0, 0, 0},
	{615, "gl.Begin", 0, 0, 0},
	{616, "gl.End", 0, 0, 0},
	{617, "gl.Normal3f", 0, 0, 0},
	{618, "gl.Vertex2f", 0, 0, 0},
	{619, "gl.Vertex3f", 0, 0, 0},
	{620, "gl.Lightfv", 0, 0, 0},
	{621, "gl.Frustum", 0, 0, 0},
	{622, "gl.TexEnvi", 0, 0, 0},
	{623, "gl.Ortho", 0, 0, 0},
	{624, "gl.Scalef", 0, 0, 0},
	{625, "gl.TexCoord2d", 0, 0, 0},
	{626, "gl.TexCoord2f", 0, 0, 0},
	{627, "gl.CullFace", 0, 0, 0},
	{628, "gl.FrontFace", 0, 0, 0},
	{629, "gl.Hint", 0, 0, 0},
	{630, "gl.Scissor", 0, 0, 0},
	{631, "gl.TexParameteri", 0, 0, 0},
	{632, "gl.TexImage2D", 0, 0, 0},
	{633, "gl.Clear", 0, 0, 0},
	{634, "gl.ClearColor", 0, 0, 0},
	{635, "gl.ClearStencil", 0, 0, 0},
	{636, "gl.ClearDepth", 0, 0, 0},
	{637, "gl.StencilMask", 0, 0, 0},
	{638, "gl.ColorMask", 0, 0, 0},
	{639, "gl.DepthMask", 0, 0, 0},
	{640, "gl.Disable", 0, 0, 0},
	{641, "gl.Enable", 0, 0, 0},
	{642, "gl.BlendFunc", 0, 0, 0},
	{643, "gl.StencilFunc", 0, 0, 0},
	{644, "gl.StencilOp", 0, 0, 0},
	{645, "gl.DepthFunc", 0, 0, 0},
	{646, "gl.GetError", 0, 0, 0},
	{647, "gl.GetTexLevelParameteriv", 0, 0, 0},
	{648, "gl.DepthRange", 0, 0, 0},
	{649, "gl.Viewport", 0, 0, 0},
	{650, "gl.DrawArrays", 0, 0, 0},
	{651, "gl.DrawElements", 0, 0, 0},
	{652, "gl.BindTexture", 0, 0, 0},
	{653, "gl.DeleteTextures", 0, 0, 0},
	{654, "gl.GenTextures", 0, 0, 0},
	{655, "gl.ActiveTexture", 0, 0, 0},
	{656, "gl.BlendFuncSeparate", 0, 0, 0},
	{657, "gl.BindBuffer", 0, 0, 0},
	{658, "gl.DeleteBuffers", 0, 0, 0},
	{659, "gl.GenBuffers", 0, 0, 0},
	{660, "gl.BufferData", 0, 0, 0},
	{661, "gl.BufferSubData", 0, 0, 0},
	{662, "gl.DrawBuffers", 0, 0, 0},
	{663, "gl.StencilOpSeparate", 0, 0, 0},
	{664, "gl.StencilFuncSeparate", 0, 0, 0},
	{665, "gl.StencilMaskSeparate", 0, 0, 0},
	{666, "gl.AttachShader", 0, 0, 0},
	{667, "gl.BindAttribLocation", 0, 0, 0},
	{668, "gl.CompileShader", 0, 0, 0},
	{669, "gl.CreateProgram", 0, 0, 0},
	{670, "gl.CreateShader", 0, 0, 0},
	{671, "gl.DeleteProgram", 0, 0, 0},
	{672, "gl.DeleteShader", 0, 0, 0},
	{673, "gl.DetachShader", 0, 0, 0},
	{674, "gl.EnableVertexAttribArray", 0, 0, 0},
	{675, "gl.GetAttribLocation", 0, 0, 0},
	{676, "gl.GetProgramiv", 0, 0, 0},
	{677, "gl.GetProgramInfoLog", 0, 0, 0},
	{678, "gl.GetShaderiv", 0, 0, 0},
	{679, "gl.GetShaderInfoLog", 0, 0, 0},
	{680, "gl.GetUniformLocation", 0, 0, 0},
	{681, "gl.LinkProgram", 0, 0, 0},
	{682, "gl.ShaderSource", 0, 0, 0},
	{683, "gl.UseProgram", 0, 0, 0},
	{684, "gl.Uniform1f", 0, 0, 0},
	{685, "gl.Uniform2f", 0, 0, 0},
	{686, "gl.Uniform3f", 0, 0, 0},
	{687, "gl.Uniform4f", 0, 0, 0},
	{688, "gl.Uniform1i", 0, 0, 0},
	{689, "gl.Uniform2i", 0, 0, 0},
	{690, "gl.Uniform3i", 0, 0, 0},
	{691, "gl.Uniform4i", 0, 0, 0},
	{692, "gl.Uniform1fv", 0, 0, 0},
	{693, "gl.Uniform2fv", 0, 0, 0},
	{694, "gl.Uniform3fv", 0, 0, 0},
	{695, "gl.Uniform4fv", 0, 0, 0},
	{696, "gl.Uniform1iv", 0, 0, 0},
	{697, "gl.Uniform2iv", 0, 0, 0},
	{698, "gl.Uniform3iv", 0, 0, 0},
	{699, "gl.Uniform4iv", 0, 0, 0},
	{700, "gl.UniformMatrix2fv", 0, 0, 0},
	{701, "gl.UniformMatrix3fv", 0, 0, 0},
	{702, "gl.UniformMatrix4fv", 0, 0, 0},
	{703, "gl.UniformV4F", 0, 0, 0},
	{704, "gl.UniformM44F", 0, 0, 0},
	{705, "gl.UniformM44FV", 0, 0, 0},
	{706, "gl.VertexAttribPointer", 0, 0, 0},
	{707, "gl.VertexAttribPointerI32", 0, 0, 0},
	{708, "gl.ClearBufferI", 0, 0, 0},
	{709, "gl.ClearBufferUI", 0, 0, 0},
	{710, "gl.ClearBufferF", 0, 0, 0},
	{711, "gl.BindRenderbuffer", 0, 0, 0},
	{712, "gl.DeleteRenderbuffers", 0, 0, 0},
	{713, "gl.GenRenderbuffers", 0, 0, 0},
	{714, "gl.RenderbufferStorage", 0, 0, 0},
	{715, "gl.BindFramebuffer", 0, 0, 0},
	{716, "gl.DeleteFramebuffers", 0, 0, 0},
	{717, "gl.GenFramebuffers", 0, 0, 0},
	{718, "gl.CheckFramebufferStatus", 0, 0, 0},
	{719, "gl.FramebufferTexture2D", 0, 0, 0},
	{720, "gl.FramebufferRenderbuffer", 0, 0, 0},
	{721, "gl.GenerateMipmap", 0, 0, 0},
	{722, "gl.BindVertexArray", 0, 0, 0},
	{723, "gl.BindVertexArrayCore", 0, 0, 0},
	{724, "gl.DeleteVertexArrays", 0, 0, 0},
	{725, "gl.DeleteVertexArraysCore", 0, 0, 0},
	{726, "gl.GenVertexArrays", 0, 0, 0},
	{727, "gl.GenVertexArraysCore", 0, 0, 0},
	{728, "glfw.Fullscreen", 0, 0, 0},
	{729, "glfw.Init", 0, 0, 0},
	{730, "glfw.WindowHint", 0, 0, 0},
	{731, "glfw.CreateWindow", 0, 0, 0},
	{732, "glfw.MakeContextCurrent", 0, 0, 0},
	{733, "glfw.ShouldClose", 0, 0, 0},
	{734, "glfw.SetShouldClose", 0, 0, 0},
	{735, "glfw.PollEvents", 0, 0, 0},
	{736, "glfw.SwapBuffers", 0, 0, 0},
	{737, "glfw.GetFramebufferSize", 0, 0, 0},
	{738, "glfw.GetWindowPos", 0, 0, 0},
	{739, "glfw.GetWindowSize", 0, 0, 0},
	{740, "glfw.GetTime", 0, 0, 0},
	{741, "glfw.SwapInterval", 0, 0, 0},
	{742, "glfw.SetStartCallback", 0, 0, 0},
	{743, "glfw.SetStopCallback", 0, 0, 0},
	{744, "glfw.SetKeyboardCallback", 0, 0, 0},
	{745, "glfw.SetMouseCallback", 0, 0, 0},
	{746, "glfw.SetFramebufferSizeCallback", 0, 0, 0},
	{747, "glfw.SetWindowPosCallback", 0, 0, 0},
	{748, "glfw.SetWindowSizeCallback", 0, 0, 0},
	{749, "glfw.SetKeyCallback", 0, 0, 0},
	{750, "glfw.SetMouseButtonCallback", 0, 0, 0},
	{751, "glfw.SetCursorPosCallback", 0, 0, 0},
	{752, "glfw.GetCursorPos", 0, 0, 0},
	{753, "glfw.SetInputMode", 0, 0, 0},
	{754, "glfw.SetWindowPos", 0, 0, 0},
	{755, "glfw.GetKey", 0, 0, 0},
	{756, "glfw.RegisterCallbackI32I32", 0, 0, 0},
	{757, "glfw.InvokeCallbackI32I32", 0, 0, 0},
	{758, "glfw.GetWindowContentScale", 0, 0, 0},
	{759, "glfw.GetMonitorContentScale", 0, 0, 0},
	{760, "gltext.LoadTrueType", 0, 0, 0},
	{761, "gltext.LoadTrueTypeCore", 0, 0, 0},
	{762, "gltext.Printf", 0, 0, 0},
	{763, "gltext.Metrics", 0, 0, 0},
	{764, "gltext.Texture", 0, 0, 0},
	{765, "gltext.NextGlyph", 0, 0, 0},
	{766, "gltext.GlyphBounds", 0, 0, 0},
	{767, "gltext.GlyphMetrics", 0, 0, 0},
	{768, "gltext.GlyphInfo", 0, 0, 0},
	{769, "al.LoadWav", 0, 0, 0},
	{770, "al.CloseDevice", 0, 0, 0},
	{771, "al.DeleteBuffers", 0, 0, 0},
	{772, "al.DeleteSources", 0, 0, 0},
	{773, "al.DeviceError", 0, 0, 0},
	{774, "al.Error", 0, 0, 0},
	{775, "al.Extensions", 0, 0, 0},
	{776, "al.OpenDevice", 0, 0, 0},
	{777, "al.PauseSources", 0, 0, 0},
	{778, "al.PlaySources", 0, 0, 0},
	{779, "al.Renderer", 0, 0, 0},
	{780, "al.RewindSources", 0, 0, 0},
	{781, "al.StopSources", 0, 0, 0},
	{782, "al.Vendor", 0, 0, 0},
	{783, "al.Version", 0, 0, 0},
	{784, "al.GenBuffers", 0, 0, 0},
	{785, "al.BufferData", 0, 0, 0},
	{786, "al.GenSources", 0, 0, 0},
	{787, "al.SourceBuffersProcessed", 0, 0, 0},
	{788, "al.SourceBuffersQueued", 0, 0, 0},
	{789, "al.SourceQueueBuffers", 0, 0, 0},
	{790, "al.SourceState", 0, 0, 0},
	{791, "al.SourceUnqueueBuffers", 0, 0, 0},
	{792, "string.ByteLength", 2611200002, 0, 0},
	{793, "string.BytesOfString", 2611200002, 0, 0},
	{794, "string.StringOfBytes", 2611200002, 0, 0},
	{795, "LibraryCall", 2611230001, 0, 0},
}

// OpcodeLifecycle is one row of OpcodeLifecycleTable.
type OpcodeLifecycle struct {
	Number              int
	Name                string
	VersionAdded        int64
	VersionDeprecated   int64
	VersionReassignable int64
}

// OpcodeLifecycleLiveByName returns the live row named name.
func OpcodeLifecycleLiveByName(name string) (OpcodeLifecycle, bool) {
	for _, row := range OpcodeLifecycleTable {
		if row.Name == name && row.VersionDeprecated == 0 {
			return row, true
		}
	}
	return OpcodeLifecycle{}, false
}

// OpcodeLifecycleLiveByNumber returns the live row numbered number.
func OpcodeLifecycleLiveByNumber(number int) (OpcodeLifecycle, bool) {
	for _, row := range OpcodeLifecycleTable {
		if row.Number == number && row.VersionDeprecated == 0 {
			return row, true
		}
	}
	return OpcodeLifecycle{}, false
}

// OpcodeRenameTable is every opcode rename, in version order. A renamed opcode
// keeps its number and its row in OpcodeLifecycleTable, which holds the new
// name, and the old name cannot be called. Read only.
var OpcodeRenameTable = [...]OpcodeRename{
	// number, old name, new name, version renamed
	{561, "str.concat", "string.Concatenation", 2611200002},
	{562, "str.i8", "string.ParseDecimalI8", 2611200002},
	{563, "str.i16", "string.ParseDecimalI16", 2611200002},
	{564, "str.i32", "string.ParseDecimalI32", 2611200002},
	{565, "str.i64", "string.ParseDecimalI64", 2611200002},
	{566, "str.ui8", "string.ParseDecimalUI8", 2611200002},
	{567, "str.ui16", "string.ParseDecimalUI16", 2611200002},
	{568, "str.ui32", "string.ParseDecimalUI32", 2611200002},
	{569, "str.ui64", "string.ParseDecimalUI64", 2611200002},
	{570, "str.f32", "string.ParseFloatF32", 2611200002},
	{571, "str.f64", "string.ParseFloatF64", 2611200002},
	{572, "str.print", "string.PrintLine", 2611200002},
	{573, "str.substr", "string.SubstringByByteRange", 2611200002},
	{574, "str.index", "string.ByteIndexOfFirstSubstring", 2611200002},
	{575, "str.lastindex", "string.ByteIndexOfLastSubstring", 2611200002},
	{576, "str.trimspace", "string.TrimSurroundingWhitespace", 2611200002},
	{586, "str.equal_by_value", "string.EqualityByByteValue", 2611200002},
	{587, "str.equal_by_pointer", "string.EqualityByHeapPointer", 2611200002},
	{588, "str.compare_by_value", "string.OrderingByByteValue", 2611200002},
	{756, "glfw.func_i32_i32", "glfw.RegisterCallbackI32I32", 2611200002},
	{757, "glfw.call_i32_i32", "glfw.InvokeCallbackI32I32", 2611200002},
}

// OpcodeRename is one row of OpcodeRenameTable.
type OpcodeRename struct {
	Number  int
	OldName string
	NewName string
	Version int64
}

// OpcodeRenamed follows the renames of the former opcode name old and returns
// the last one, so its NewName is the current name.
func OpcodeRenamed(old string) (OpcodeRename, bool) {
	var last OpcodeRename
	found := false
	for _, rename := range OpcodeRenameTable {
		if rename.OldName == old || (found && rename.OldName == last.NewName && rename.Number == last.Number) {
			last, found = rename, true
		}
	}
	return last, found
}

// opcodeRenamesOf returns the renames of row in version order.
func opcodeRenamesOf(row OpcodeLifecycle) []OpcodeRename {
	var renames []OpcodeRename
	for _, rename := range OpcodeRenameTable {
		if rename.Number == row.Number && rename.Version > row.VersionAdded && (row.VersionDeprecated == 0 || rename.Version <= row.VersionDeprecated) {
			renames = append(renames, rename)
		}
	}
	return renames
}

// OpcodeNameAt returns the name row had in version.
func OpcodeNameAt(row OpcodeLifecycle, version int64) string {
	for _, rename := range opcodeRenamesOf(row) {
		if rename.Version > version {
			return rename.OldName
		}
	}
	return row.Name
}

// Named opcodes. Each value is its number in OpcodeLifecycleTable. The
// START_ and END_ markers bound the operator ranges and are not opcodes.
// nolint golint
const (
	OP_IDENTITY          = 1
	OP_JMP               = 2
	OP_ABS_JMP           = 3
	OP_JMP_EQ            = 4
	OP_JMP_UNEQ          = 5
	OP_JMP_GT            = 6
	OP_JMP_GTEQ          = 7
	OP_JMP_LT            = 8
	OP_JMP_LTEQ          = 9
	OP_JMP_ZERO          = 10
	OP_JMP_NOT_ZERO      = 11
	OP_NOP               = 12
	OP_GOTO              = 13
	OP_BREAK             = 14
	OP_CONTINUE          = 15
	OP_DEBUG_PRINT_STACK = 16
	OP_SERIALIZE         = 17
	OP_DESERIALIZE       = 18

	START_OF_OPERATORS            = 19
	START_OF_COMPARISON_OPERATORS = 38
	OP_EQUAL                      = 39
	OP_UNEQUAL                    = 40
	OP_LT                         = 41
	OP_GT                         = 42
	OP_LTEQ                       = 43
	OP_GTEQ                       = 44
	END_OF_COMPARISON_OPERATORS   = 45
	START_OF_ARITHMETIC_OPERATORS = 72
	OP_BITAND                     = 73
	OP_BITOR                      = 74
	OP_BITXOR                     = 75
	OP_BITCLEAR                   = 76
	OP_BITSHL                     = 77
	OP_BITSHR                     = 78
	OP_MUL                        = 79
	OP_DIV                        = 80
	OP_MOD                        = 81
	OP_ADD                        = 82
	OP_SUB                        = 83
	OP_NEG                        = 84
	END_OF_ARITHMETIC_OPERATORS   = 85
	END_OF_OPERATORS              = 126

	OP_ASSERT            = 127
	OP_TEST              = 128
	OP_PANIC             = 129
	OP_APPEND            = 130
	OP_BOOL_OR           = 131
	OP_BOOL_AND          = 132
	OP_BOOL_NOT          = 133
	END_OF_NAMED_OPCODES = 134

	// OP_LIBRARY_CALL calls function CXFunction.LibraryFunctionNumber of system
	// library CXFunction.LibraryNumber, see libraries.go.
	OP_LIBRARY_CALL = 795

	// The slice built-ins after append, which the compiler checks by number.
	OP_RESIZE = 210
	OP_INSERT = 211
	OP_REMOVE = 212
	OP_COPY   = 213
)

var (
	// VIOLATION: state outside State. Owner: t1-constants; moves to CXNativeFunctionMng in phase 1.
	START_PARSE_OPS int
	// VIOLATION: state outside State. Owner: t1-constants; moves to CXNativeFunctionMng in phase 1.
	END_PARSE_OPS int
)
