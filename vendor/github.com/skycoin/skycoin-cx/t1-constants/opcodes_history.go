package constants

// Opcode lifecycle events in version order, see opcodes.go for the conventions.
//
// version     event         number name (a rename lists the old name, then the new)
// 0000000000  assigned      every row of OpcodeLifecycleTable with version added 0
// 2611200001  assigned      586 str.equal_by_value
// 2611200001  assigned      587 str.equal_by_pointer
// 2611200001  assigned      588 str.compare_by_value
// 2611200001  assigned      589 ptr.eq
// 2611200001  assigned      590 ptr.uneq
// 2611200001  deprecated    559 str.eq
// 2611200001  deprecated    560 str.uneq
// 2611200001  deprecated    582 str.lt
// 2611200001  deprecated    583 str.lteq
// 2611200001  deprecated    584 str.gt
// 2611200001  deprecated    585 str.gteq
// 2611200002  assigned      792 string.ByteLength
// 2611200002  assigned      793 string.BytesOfString
// 2611200002  assigned      794 string.StringOfBytes
// 2611200002  renamed       561 str.concat string.Concatenation
// 2611200002  renamed       562 str.i8 string.ParseDecimalI8
// 2611200002  renamed       563 str.i16 string.ParseDecimalI16
// 2611200002  renamed       564 str.i32 string.ParseDecimalI32
// 2611200002  renamed       565 str.i64 string.ParseDecimalI64
// 2611200002  renamed       566 str.ui8 string.ParseDecimalUI8
// 2611200002  renamed       567 str.ui16 string.ParseDecimalUI16
// 2611200002  renamed       568 str.ui32 string.ParseDecimalUI32
// 2611200002  renamed       569 str.ui64 string.ParseDecimalUI64
// 2611200002  renamed       570 str.f32 string.ParseFloatF32
// 2611200002  renamed       571 str.f64 string.ParseFloatF64
// 2611200002  renamed       572 str.print string.PrintLine
// 2611200002  renamed       573 str.substr string.SubstringByByteRange
// 2611200002  renamed       574 str.index string.ByteIndexOfFirstSubstring
// 2611200002  renamed       575 str.lastindex string.ByteIndexOfLastSubstring
// 2611200002  renamed       576 str.trimspace string.TrimSurroundingWhitespace
// 2611200002  renamed       586 str.equal_by_value string.EqualityByByteValue
// 2611200002  renamed       587 str.equal_by_pointer string.EqualityByHeapPointer
// 2611200002  renamed       588 str.compare_by_value string.OrderingByByteValue
// 2611200002  renamed       756 glfw.func_i32_i32 glfw.RegisterCallbackI32I32
// 2611200002  renamed       757 glfw.call_i32_i32 glfw.InvokeCallbackI32I32
// 2611230001  assigned      795 LibraryCall
// 2611230001  deprecated    138 os.GetWorkingDirectory
// 2611230001  deprecated    139 os.LogFile
// 2611230001  deprecated    140 os.Open
// 2611230001  deprecated    141 os.Create
// 2611230001  deprecated    142 os.Close
// 2611230001  deprecated    143 os.Seek
// 2611230001  deprecated    144 os.ReadStr
// 2611230001  deprecated    145 os.ReadF64
// 2611230001  deprecated    146 os.ReadF32
// 2611230001  deprecated    147 os.ReadUI64
// 2611230001  deprecated    148 os.ReadUI32
// 2611230001  deprecated    149 os.ReadUI16
// 2611230001  deprecated    150 os.ReadUI8
// 2611230001  deprecated    151 os.ReadI64
// 2611230001  deprecated    152 os.ReadI32
// 2611230001  deprecated    153 os.ReadI16
// 2611230001  deprecated    154 os.ReadI8
// 2611230001  deprecated    155 os.ReadBOOL
// 2611230001  deprecated    156 os.ReadF64Slice
// 2611230001  deprecated    157 os.ReadF32Slice
// 2611230001  deprecated    158 os.ReadUI64Slice
// 2611230001  deprecated    159 os.ReadUI32Slice
// 2611230001  deprecated    160 os.ReadUI16Slice
// 2611230001  deprecated    161 os.ReadUI8Slice
// 2611230001  deprecated    162 os.ReadI64Slice
// 2611230001  deprecated    163 os.ReadI32Slice
// 2611230001  deprecated    164 os.ReadI16Slice
// 2611230001  deprecated    165 os.ReadI8Slice
// 2611230001  deprecated    166 os.ReadAllText
// 2611230001  deprecated    167 os.WriteStr
// 2611230001  deprecated    168 os.WriteF64
// 2611230001  deprecated    169 os.WriteF32
// 2611230001  deprecated    170 os.WriteUI64
// 2611230001  deprecated    171 os.WriteUI32
// 2611230001  deprecated    172 os.WriteUI16
// 2611230001  deprecated    173 os.WriteUI8
// 2611230001  deprecated    174 os.WriteI64
// 2611230001  deprecated    175 os.WriteI32
// 2611230001  deprecated    176 os.WriteI16
// 2611230001  deprecated    177 os.WriteI8
// 2611230001  deprecated    178 os.WriteBOOL
// 2611230001  deprecated    179 os.WriteF64Slice
// 2611230001  deprecated    180 os.WriteF32Slice
// 2611230001  deprecated    181 os.WriteUI64Slice
// 2611230001  deprecated    182 os.WriteUI32Slice
// 2611230001  deprecated    183 os.WriteUI16Slice
// 2611230001  deprecated    184 os.WriteUI8Slice
// 2611230001  deprecated    185 os.WriteI64Slice
// 2611230001  deprecated    186 os.WriteI32Slice
// 2611230001  deprecated    187 os.WriteI16Slice
// 2611230001  deprecated    188 os.WriteI8Slice
// 2611230001  deprecated    189 os.Run
// 2611230001  deprecated    191 json.Open
// 2611230001  deprecated    192 json.Close
// 2611230001  deprecated    193 json.More
// 2611230001  deprecated    194 json.Next
// 2611230001  deprecated    195 json.Type
// 2611230001  deprecated    196 json.Delim
// 2611230001  deprecated    197 json.Bool
// 2611230001  deprecated    198 json.Float64
// 2611230001  deprecated    199 json.Int64
// 2611230001  deprecated    200 json.Str
// 2611230001  deprecated    201 StartCPUProfile
// 2611230001  deprecated    202 StopCPUProfile
// 2611230001  deprecated    206 tcp.Dial
// 2611230001  deprecated    207 tcp.Listen
// 2611230001  deprecated    208 tcp.Accept
// 2611230001  deprecated    209 tcp.Close
// 2711200001  reassignable  559 str.eq
// 2711200001  reassignable  560 str.uneq
// 2711200001  reassignable  582 str.lt
// 2711200001  reassignable  583 str.lteq
// 2711200001  reassignable  584 str.gt
// 2711200001  reassignable  585 str.gteq
// 2711230001  reassignable  138 os.GetWorkingDirectory
// 2711230001  reassignable  139 os.LogFile
// 2711230001  reassignable  140 os.Open
// 2711230001  reassignable  141 os.Create
// 2711230001  reassignable  142 os.Close
// 2711230001  reassignable  143 os.Seek
// 2711230001  reassignable  144 os.ReadStr
// 2711230001  reassignable  145 os.ReadF64
// 2711230001  reassignable  146 os.ReadF32
// 2711230001  reassignable  147 os.ReadUI64
// 2711230001  reassignable  148 os.ReadUI32
// 2711230001  reassignable  149 os.ReadUI16
// 2711230001  reassignable  150 os.ReadUI8
// 2711230001  reassignable  151 os.ReadI64
// 2711230001  reassignable  152 os.ReadI32
// 2711230001  reassignable  153 os.ReadI16
// 2711230001  reassignable  154 os.ReadI8
// 2711230001  reassignable  155 os.ReadBOOL
// 2711230001  reassignable  156 os.ReadF64Slice
// 2711230001  reassignable  157 os.ReadF32Slice
// 2711230001  reassignable  158 os.ReadUI64Slice
// 2711230001  reassignable  159 os.ReadUI32Slice
// 2711230001  reassignable  160 os.ReadUI16Slice
// 2711230001  reassignable  161 os.ReadUI8Slice
// 2711230001  reassignable  162 os.ReadI64Slice
// 2711230001  reassignable  163 os.ReadI32Slice
// 2711230001  reassignable  164 os.ReadI16Slice
// 2711230001  reassignable  165 os.ReadI8Slice
// 2711230001  reassignable  166 os.ReadAllText
// 2711230001  reassignable  167 os.WriteStr
// 2711230001  reassignable  168 os.WriteF64
// 2711230001  reassignable  169 os.WriteF32
// 2711230001  reassignable  170 os.WriteUI64
// 2711230001  reassignable  171 os.WriteUI32
// 2711230001  reassignable  172 os.WriteUI16
// 2711230001  reassignable  173 os.WriteUI8
// 2711230001  reassignable  174 os.WriteI64
// 2711230001  reassignable  175 os.WriteI32
// 2711230001  reassignable  176 os.WriteI16
// 2711230001  reassignable  177 os.WriteI8
// 2711230001  reassignable  178 os.WriteBOOL
// 2711230001  reassignable  179 os.WriteF64Slice
// 2711230001  reassignable  180 os.WriteF32Slice
// 2711230001  reassignable  181 os.WriteUI64Slice
// 2711230001  reassignable  182 os.WriteUI32Slice
// 2711230001  reassignable  183 os.WriteUI16Slice
// 2711230001  reassignable  184 os.WriteUI8Slice
// 2711230001  reassignable  185 os.WriteI64Slice
// 2711230001  reassignable  186 os.WriteI32Slice
// 2711230001  reassignable  187 os.WriteI16Slice
// 2711230001  reassignable  188 os.WriteI8Slice
// 2711230001  reassignable  189 os.Run
// 2711230001  reassignable  191 json.Open
// 2711230001  reassignable  192 json.Close
// 2711230001  reassignable  193 json.More
// 2711230001  reassignable  194 json.Next
// 2711230001  reassignable  195 json.Type
// 2711230001  reassignable  196 json.Delim
// 2711230001  reassignable  197 json.Bool
// 2711230001  reassignable  198 json.Float64
// 2711230001  reassignable  199 json.Int64
// 2711230001  reassignable  200 json.Str
// 2711230001  reassignable  201 StartCPUProfile
// 2711230001  reassignable  202 StopCPUProfile
// 2711230001  reassignable  206 tcp.Dial
// 2711230001  reassignable  207 tcp.Listen
// 2711230001  reassignable  208 tcp.Accept
// 2711230001  reassignable  209 tcp.Close
//
// Interval format. A version is yymmddnnnn, so version / 100000000 is the
// year since 2000, the next two digits the month, the next two the day and
// the last four the release on that day. The reassignable version of a
// deprecation is the same month and day OpcodeReassignmentIntervalYears years
// later, release 0001. Deprecated 2611200001 (2026-11-20 release 0001) is
// reassignable from 2711200001 (2027-11-20 release 0001). A 29 February
// deprecation without a 29 February in the target year moves to 1 March.
// TestOpcodeLifecycleHistoryMatchesTable fails when this list and the table
// differ.

// OpcodeReassignmentIntervalYears is how long a deprecated opcode number
// stays unassigned.
const OpcodeReassignmentIntervalYears = 1

// LanguageVersionReassignable returns the version from which an opcode number
// deprecated in version deprecated may be assigned again.
func LanguageVersionReassignable(deprecated int64) int64 {
	year, month, day := languageVersionDate(deprecated)
	return languageVersionFromDate(year+OpcodeReassignmentIntervalYears, month, day, 1)
}

// LanguageVersionValid reports whether version is a yymmddnnnn with a real
// date and a release number of at least 1.
func LanguageVersionValid(version int64) bool {
	year, month, day := languageVersionDate(version)
	release := version % 10000
	if version <= 0 || version > 9912319999 || release < 1 || month < 1 || month > 12 || day < 1 {
		return false
	}
	return day <= languageVersionDaysInMonth(year, month)
}

func languageVersionDate(version int64) (year, month, day int) {
	return int(version / 100000000), int(version / 1000000 % 100), int(version / 10000 % 100)
}

func languageVersionFromDate(year, month, day int, release int64) int64 {
	if day > languageVersionDaysInMonth(year, month) {
		month, day = month+1, 1
	}
	return int64(year)*100000000 + int64(month)*1000000 + int64(day)*10000 + release
}

func languageVersionDaysInMonth(year, month int) int {
	switch month {
	case 2:
		if (2000+year)%4 == 0 && ((2000+year)%100 != 0 || (2000+year)%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}
