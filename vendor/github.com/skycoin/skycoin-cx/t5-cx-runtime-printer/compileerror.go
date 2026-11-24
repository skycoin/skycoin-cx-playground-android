package printer

import (
	"os"
	"sort"
	"strconv"

	cxpackages "github.com/skycoin/skycoin-cx/t0-packages"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// printerCompileErrorFormat is the text of one compile error code. In
// Format, %H is the "error: file:line:column" header, without a column of 0,
// %s the next operand as a StringGid and %d the next as an integer. %P is
// the next three operands as file:line:column, without a column of 0. %+ is
// "s" when the last integer is above 1, and %~ is " was" when it is 1 and
// "s were" otherwise.
// Stdout sends the text to stdout instead of stderr. Warning makes %H
// "warning: " for a record that does not fail the build.
type printerCompileErrorFormat struct {
	Format  string
	Stdout  bool
	Warning bool
}

var printerCompileErrorFormats = [ast.CompileErrorCodeEnd - ast.CompileErrorCodeNil]printerCompileErrorFormat{
	ast.CompileErrorCodeMessage - ast.CompileErrorCodeNil:                         {Format: "%H %s\n"},
	ast.CompileErrorCodeAssignOutputlessOperator - ast.CompileErrorCodeNil:        {Format: "%H trying to use an outputless operator in an assignment\n"},
	ast.CompileErrorCodeAssignSymbolType - ast.CompileErrorCodeNil:                {Format: "%H trying to assign argument of type '%s' to symbol '%s' of type '%s'\n"},
	ast.CompileErrorCodeAssignValueType - ast.CompileErrorCodeNil:                 {Format: "%H cannot assign value of type '%s' to identifier '%s' of type '%s'\n"},
	ast.CompileErrorCodeBuiltinPackageNotImported - ast.CompileErrorCodeNil:       {Format: "%H built-in package %s is used without importing it, add import \"" + cxpackages.SysPrefix + "%s\"\n"},
	ast.CompileErrorCodeFunctionInputType - ast.CompileErrorCodeNil:               {Format: "%H function '%s' expected input argument of type '%s'; '%s' was provided\n"},
	ast.CompileErrorCodeFunctionMissing - ast.CompileErrorCodeNil:                 {Format: "%H function '%s' does not exist\n"},
	ast.CompileErrorCodeFunctionReceiverType - ast.CompileErrorCodeNil:            {Format: "%H function '%s' expected receiving variable of type '%s'; '%s' was provided\n"},
	ast.CompileErrorCodeRedeclaredBlock - ast.CompileErrorCodeNil:                 {Format: "%H '%s' redeclared in this block, other declaration at %P\n"},
	ast.CompileErrorCodeFunctionReturnCount - ast.CompileErrorCodeNil:             {Format: "%H function '%s' expects to return %d argument%+, but %d output argument%~ provided\n"},
	ast.CompileErrorCodeIdentifierMissing - ast.CompileErrorCodeNil:               {Format: "%H identifier '%s' does not exist\n"},
	ast.CompileErrorCodeIndexType - ast.CompileErrorCodeNil:                       {Format: "%H wrong index type; expected either 'i32' or 'i64', got '%s'\n"},
	ast.CompileErrorCodeInputTypesNotEqual - ast.CompileErrorCodeNil:              {Format: "%H first and second input arguments' types are not equal in '%s' call ('%s' != '%s')\n"},
	ast.CompileErrorCodeInvalidExpression - ast.CompileErrorCodeNil:               {Format: "%H invalid expression\n"},
	ast.CompileErrorCodeInvalidIndexing - ast.CompileErrorCodeNil:                 {Format: "%H invalid indexing\n"},
	ast.CompileErrorCodeInvalidIndirection - ast.CompileErrorCodeNil:              {Format: "%H invalid indirection\n"},
	ast.CompileErrorCodeLoopConditionNoValue - ast.CompileErrorCodeNil:            {Format: "%H loop condition has no value\n"},
	ast.CompileErrorCodeNoFields - ast.CompileErrorCodeNil:                        {Format: "%H '%s' has no fields\n"},
	ast.CompileErrorCodeNoPackageDefined - ast.CompileErrorCodeNil:                {Format: "%H no package clause\n"},
	ast.CompileErrorCodeOpcodeRenamed - ast.CompileErrorCodeNil:                   {Format: "%H %s was renamed to %s in language version %d\n"},
	ast.CompileErrorCodeOpcodeRenamedImport - ast.CompileErrorCodeNil:             {Format: "%H %s was renamed to %s in language version %d, import \"" + cxpackages.SysPrefix + "%s\"\n"},
	ast.CompileErrorCodeOperatorArgumentTypes - ast.CompileErrorCodeNil:           {Format: "%H invalid argument types for '%s' operator\n"},
	ast.CompileErrorCodeOperatorInputCount - ast.CompileErrorCodeNil:              {Format: "%H operator '%s' expects %d input/s, but %d input argument%~ provided\n"},
	ast.CompileErrorCodeOperatorOutputCount - ast.CompileErrorCodeNil:             {Format: "%H operator '%s' expects to return %d output/s, but %d receiving argument/s were provided\n"},
	ast.CompileErrorCodePackageIdentifierMissing - ast.CompileErrorCodeNil:        {Format: "%H identifier '%s.%s' does not exist\n"},
	ast.CompileErrorCodePointerOrderingOperator - ast.CompileErrorCodeNil:         {Format: "%H pointer %s pointer is not allowed, pointers support only == and !=\n"},
	ast.CompileErrorCodePrimitiveFieldAccess - ast.CompileErrorCodeNil:            {Format: "%H illegal method call or field access on identifier '%s' of primitive type '%s'\n"},
	ast.CompileErrorCodeRedeclared - ast.CompileErrorCodeNil:                      {Format: "%H '%s' redeclared\n"},
	ast.CompileErrorCodeStrArithmeticOperator - ast.CompileErrorCodeNil:           {Format: "%H operator %s is not allowed on str, strings have no arithmetic operators\n"},
	ast.CompileErrorCodeStrConcatenation - ast.CompileErrorCodeNil:                {Format: "%H str + str is not allowed, use string.Concatenation\n"},
	ast.CompileErrorCodeStrEqualityOperator - ast.CompileErrorCodeNil:             {Format: "%H str %s str is not allowed, use string.EqualityByByteValue or string.EqualityByHeapPointer\n"},
	ast.CompileErrorCodeStrOrderingOperator - ast.CompileErrorCodeNil:             {Format: "%H str %s str is not allowed, use string.OrderingByByteValue\n"},
	ast.CompileErrorCodeStructFieldDuplicate - ast.CompileErrorCodeNil:            {Format: "%H duplicate field %s\n"},
	ast.CompileErrorCodeStructFieldMultiplyDefined - ast.CompileErrorCodeNil:      {Format: "%H Multiply defined struct field: %s\n"},
	ast.CompileErrorCodeStructLiteralFieldType - ast.CompileErrorCodeNil:          {Format: "%H field '%s' in struct literal of type '%s' expected argument of type '%s'; '%s' was provided\n"},
	ast.CompileErrorCodeMethodMissing - ast.CompileErrorCodeNil:                   {Format: "%H type '%s' has no method '%s'\n"},
	ast.CompileErrorCodeDeclarationTooLarge - ast.CompileErrorCodeNil:             {Format: "%H '%s' is too large\n"},
	ast.CompileErrorCodeStructRecursive - ast.CompileErrorCodeNil:                 {Format: "%H invalid recursive type '%s'\n"},
	ast.CompileErrorCodeLoopJumpOutsideLoop - ast.CompileErrorCodeNil:             {Format: "%H %s is not in a loop\n"},
	ast.CompileErrorCodeSliceArgumentType - ast.CompileErrorCodeNil:               {Format: "%H function '%s' expected a slice as input argument; '%s' was provided\n"},
	ast.CompileErrorCodeGotoLabelMissing - ast.CompileErrorCodeNil:                {Format: "%H label '%s' is not defined\n"},
	ast.CompileErrorCodeSliceElementType - ast.CompileErrorCodeNil:                {Format: "%H function '%s' expected argument of type '%s'; '%s' was provided\n"},
	ast.CompileErrorCodeCallResultCall - ast.CompileErrorCodeNil:                  {Format: "%H the result of a call is not a function\n"},
	ast.CompileErrorCodeMethodResultMissing - ast.CompileErrorCodeNil:             {Format: "%H method '%s.%s' returns no value\n"},
	ast.CompileErrorCodeFunctionResultMissing - ast.CompileErrorCodeNil:           {Format: "%H function '%s' returns no value\n"},
	ast.CompileErrorCodeMultipleValueSingle - ast.CompileErrorCodeNil:             {Format: "%H multiple-value %s() (value of type (%s)) in single-value context\n"},
	ast.CompileErrorCodeShortDeclarationNoNew - ast.CompileErrorCodeNil:           {Format: "%H no new variables on left side of :=\n"},
	ast.CompileErrorCodeAssignmentCount - ast.CompileErrorCodeNil:                 {Format: "%H assignment mismatch: %d variable%+ but %d value%+\n"},
	ast.CompileErrorCodeArrayLiteralLength - ast.CompileErrorCodeNil:              {Format: "%H array literal of length %d has %d element%+\n"},
	ast.CompileErrorCodeStructLiteralFieldRepeated - ast.CompileErrorCodeNil:      {Format: "%H duplicate field name '%s' in struct literal\n"},
	ast.CompileErrorCodeImportGuiBuildMissing - ast.CompileErrorCodeNil:           {Format: "%H package \"%s\" needs a GUI build of cx, build it with make build-gui\n"},
	ast.CompileErrorCodeLiteralElementType - ast.CompileErrorCodeNil:              {Format: "%H invalid composite literal type '%s'\n"},
	ast.CompileErrorCodeStructLiteralFieldNames - ast.CompileErrorCodeNil:         {Format: "%H struct literal of type '%s' must name its fields\n"},
	ast.CompileErrorCodeSliceLiteralElementType - ast.CompileErrorCodeNil:         {Format: "%H cannot use value of type '%s' as type '%s' in slice literal\n"},
	ast.CompileErrorCodeSyntax - ast.CompileErrorCodeNil:                          {Format: "%H syntax error: %s\n"},
	ast.CompileErrorCodeDereferenceNonPointer - ast.CompileErrorCodeNil:           {Format: "%H cannot dereference non-pointer '%s' of type '%s'\n"},
	ast.CompileErrorCodeMethodRedeclared - ast.CompileErrorCodeNil:                {Format: "%H method %s already declared at %P\n"},
	ast.CompileErrorCodeImportFailed - ast.CompileErrorCodeNil:                    {Format: "%H could not import %s (%s)\n"},
	ast.CompileErrorCodeMethodReceiversMultiple - ast.CompileErrorCodeNil:         {Format: "%H method has multiple receivers\n"},
	ast.CompileErrorCodeMethodPointerValue - ast.CompileErrorCodeNil:              {Format: "%H cannot call pointer method %s on %s\n"},
	ast.CompileErrorCodeArrayLiteralElementType - ast.CompileErrorCodeNil:         {Format: "%H cannot use value of type '%s' as type '%s' in array literal\n"},
	ast.CompileErrorCodeReturnValueType - ast.CompileErrorCodeNil:                 {Format: "%H cannot use value of type '%s' as type '%s' in return statement\n"},
	ast.CompileErrorCodeGlobalTypeUnknown - ast.CompileErrorCodeNil:               {Format: "%H cannot infer the type of global '%s' from its value, declare it with a type\n"},
	ast.CompileErrorCodeInitializationCycle - ast.CompileErrorCodeNil:             {Format: "%H initialization cycle for '%s': %s\n"},
	ast.CompileErrorCodeGlobalUndeclared - ast.CompileErrorCodeNil:                {Format: "%H function '%s' uses global '%s' without declaring it, add global %s at the top of its body\n"},
	ast.CompileErrorCodeGlobalDeclaredUnused - ast.CompileErrorCodeNil:            {Format: "%H global '%s' declared and not used in function '%s'\n"},
	ast.CompileErrorCodeGlobalDeclarationPlacement - ast.CompileErrorCodeNil:      {Format: "%H global declarations must be the first statements of a function body\n"},
	ast.CompileErrorCodeGlobalDeclarationMissing - ast.CompileErrorCodeNil:        {Format: "%H '%s' is not a global of package '%s'\n"},
	ast.CompileErrorCodeGlobalDeclarationPackageMissing - ast.CompileErrorCodeNil: {Format: "%H package '%s' of global '%s' is not imported\n"},
	ast.CompileErrorCodeGlobalDeclarationRepeated - ast.CompileErrorCodeNil:       {Format: "%H global '%s' declared twice\n"},
	ast.CompileErrorCodeShadowGlobal - ast.CompileErrorCodeNil:                    {Format: "%H '%s' shadows global declared at %P\n"},
	ast.CompileErrorCodeShadowImport - ast.CompileErrorCodeNil:                    {Format: "%H '%s' shadows package imported at %P\n"},
	ast.CompileErrorCodeShadowFunction - ast.CompileErrorCodeNil:                  {Format: "%H '%s' shadows function declared at %P\n"},
	ast.CompileErrorCodeShadowType - ast.CompileErrorCodeNil:                      {Format: "%H '%s' shadows type declared at %P\n"},
	ast.CompileErrorCodeShadowVariable - ast.CompileErrorCodeNil:                  {Format: "%H '%s' shadows variable declared at %P\n"},
	ast.CompileErrorCodeImportMasking - ast.CompileErrorCodeNil:                   {Format: "%H '%s' masks the name imported at %P\n"},
	ast.CompileErrorCodeImportMaskingWarning - ast.CompileErrorCodeNil:            {Format: "%H '%s' masks the name imported at %P\n", Warning: true},
	ast.CompileErrorCodeImportFormInvalid - ast.CompileErrorCodeNil:               {Format: "%H invalid import: %s\n"},
	ast.CompileErrorCodeImportNameUnknown - ast.CompileErrorCodeNil:               {Format: "%H '%s' is not a global, function or type of package '%s'\n"},
}

// PrinterCompileErrorsRegister makes prgrm print its compile errors through
// this package when the compiler exits on one.
func PrinterCompileErrorsRegister(prgrm *ast.CXProgram) {
	prgrm.CompileErrorPrint = PrinterCompileErrorsPrint
}

// PrinterCompileErrorsPrint prints the compile errors of prgrm that are not
// printed yet, ordered by file, line and column, and a record equal to the one
// before it once.
func PrinterCompileErrorsPrint(prgrm *ast.CXProgram) {
	list := &prgrm.CxIrState.CompileErrorList
	printerCompileErrorsSort(list.CompileErrorListRecords[list.CompileErrorListPrinted:])
	for ; list.CompileErrorListPrinted < len(list.CompileErrorListRecords); list.CompileErrorListPrinted++ {
		record := &list.CompileErrorListRecords[list.CompileErrorListPrinted]
		if list.CompileErrorListPrinted > 0 && *record == list.CompileErrorListRecords[list.CompileErrorListPrinted-1] {
			continue
		}
		format := printerCompileErrorFormats[record.CompileErrorRecordCode-ast.CompileErrorCodeNil]
		text := PrinterCompileErrorAppend(nil, prgrm, record)
		if format.Stdout {
			os.Stdout.Write(text)
		} else {
			os.Stderr.Write(text)
		}
	}
}

// PrinterCompileErrorAppend appends the text of record to text.
func PrinterCompileErrorAppend(text []byte, prgrm *ast.CXProgram, record *ast.CompileErrorRecord) []byte {
	entry := printerCompileErrorFormats[record.CompileErrorRecordCode-ast.CompileErrorCodeNil]
	format := entry.Format
	operand := 0
	var lastInteger int64
	for index := 0; index < len(format); index++ {
		if format[index] != '%' || index+1 == len(format) {
			text = append(text, format[index])
			continue
		}
		index++
		switch format[index] {
		case 'H':
			if entry.Warning {
				text = append(text, "warning: "...)
			} else {
				text = append(text, "error: "...)
			}
			text = printerCompileErrorPositionAppend(text, prgrm, record.CompileErrorRecordFile, int64(record.CompileErrorRecordLine), int64(record.CompileErrorRecordColumn))
		case 'P':
			operands := &record.CompileErrorRecordOperands
			text = printerCompileErrorPositionAppend(text, prgrm, ast.SourceFileGid(operands[operand]), operands[operand+1], operands[operand+2])
			operand += 3
		case 's':
			text = append(text, prgrm.CXProgramNameBytes(stringpool.StringGid(record.CompileErrorRecordOperands[operand]))...)
			operand++
		case 'd':
			lastInteger = record.CompileErrorRecordOperands[operand]
			text = strconv.AppendInt(text, lastInteger, 10)
			operand++
		case '+':
			if lastInteger > 1 {
				text = append(text, 's')
			}
		case '~':
			if lastInteger == 1 {
				text = append(text, " was"...)
			} else {
				text = append(text, "s were"...)
			}
		default:
			text = append(text, '%', format[index])
		}
	}
	return text
}

// printerCompileErrorPositionAppend appends file:line:column to text, without
// a column of 0.
func printerCompileErrorPositionAppend(text []byte, prgrm *ast.CXProgram, file ast.SourceFileGid, line int64, column int64) []byte {
	text = append(text, prgrm.CXProgramSourceFileText(file)...)
	text = append(text, ':')
	text = strconv.AppendInt(text, line, 10)
	if column > 0 {
		text = append(text, ':')
		text = strconv.AppendInt(text, column, 10)
	}
	return text
}

// printerCompileErrorsSort orders pending by file, line and column, keeping
// equal positions in record order.
func printerCompileErrorsSort(pending []ast.CompileErrorRecord) {
	sort.SliceStable(pending, func(left, right int) bool {
		if pending[left].CompileErrorRecordFile != pending[right].CompileErrorRecordFile {
			return pending[left].CompileErrorRecordFile < pending[right].CompileErrorRecordFile
		}
		if pending[left].CompileErrorRecordLine != pending[right].CompileErrorRecordLine {
			return pending[left].CompileErrorRecordLine < pending[right].CompileErrorRecordLine
		}
		return pending[left].CompileErrorRecordColumn < pending[right].CompileErrorRecordColumn
	})
}
