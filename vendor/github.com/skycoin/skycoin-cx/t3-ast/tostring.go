package ast

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	cxpackages "github.com/skycoin/skycoin-cx/t0-packages" // thin: only IsDefaultPackage; no depth gain

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

// ToString returns the abstract syntax tree of a CX program in a
// string format.
func ToString(cxprogram *CXProgram) string {
	var ast3 string
	// ast3 += "Program\n" //why is top line "Program" ???

	BuildStrPackages(cxprogram, &ast3) //what does this do?

	return ast3
}

// buildStrImports is an auxiliary function for `toString`. It builds
// string representation all the imported packages of `pkg`.
func buildStrImports(prgrm *CXProgram, pkg *CXPackage, ast *string) {
	if len(pkg.Imports) > 0 {
		*ast += "\tImports\n"
	}

	count := 0
	for _, impIdx := range slices.Sorted(maps.Values(pkg.Imports)) {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0476.
		impPkg, err := prgrm.GetPackageFromArray(impIdx)
		if err != nil {
			panic(err)
		}
		*ast += fmt.Sprintf("\t\t%d.- Import: %s\n", count, prgrm.CXProgramNameText(impPkg.Name))
		count++
	}
}

// buildStrGlobals is an auxiliary function for `toString`. It builds
// string representation of all the global variables of `pkg`.
func buildStrGlobals(prgrm *CXProgram, pkg *CXPackage, ast *string) {
	if len(pkg.Globals.Fields) > 0 {
		*ast += "\tGlobals\n"
	}

	for idx, glblIdx := range pkg.Globals.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0477.
		glbl := prgrm.GetCXTypeSignatureFromArray(glblIdx)

		*ast += fmt.Sprintf("\t\t%d.- Global: %s %s\n", idx, prgrm.CXProgramNameText(glbl.Name), GetFormattedType(prgrm, glbl))
	}
}

// buildStrStructs is an auxiliary function for `toString`. It builds
// string representation of all the structures defined in `pkg`.
func buildStrStructs(prgrm *CXProgram, pkg *CXPackage, ast *string) {
	if len(pkg.Structs) > 0 {
		*ast += "\tStructs\n"
	}

	count := 0
	for _, strctIdx := range slices.Sorted(maps.Values(pkg.Structs)) {
		strct := prgrm.CXStructs[strctIdx]
		*ast += fmt.Sprintf("\t\t%d.- Struct: %s\n", count, prgrm.CXProgramNameText(strct.Name))

		for k, typeSignatureIdx := range strct.Fields {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0478.
			typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)

			*ast += fmt.Sprintf("\t\t\t%d.- Field: %s %s\n",
				k, prgrm.CXProgramNameText(typeSignature.Name), GetFormattedType(prgrm, typeSignature))

		}

		count++
	}
}

// buildStrFunctions is an auxiliary function for `toString`. It builds
// string representation of all the functions defined in `pkg`.
func buildStrFunctions(prgrm *CXProgram, pkg *CXPackage, ast1 *string) {
	if len(pkg.Functions) > 0 {
		*ast1 += "\tFunctions\n"
	}

	// We need to declare the counter outside so we can
	// ignore the increment from the `*init` function. Functions print in
	// declaration order, the order of their indexes.
	var j int
	for _, fnIdx := range slices.Sorted(maps.Values(pkg.Functions)) {
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0479.
		fn := prgrm.GetFunctionFromArray(fnIdx)

		if fn.Name == prgrm.CxIrState.CxIrStateNameSysInit {
			continue
		}
		_, err := pkg.SelectFunction(prgrm, fn.Name)
		if err != nil {
			panic(err)
		}

		var inps bytes.Buffer
		var outs bytes.Buffer

		getFormattedParam(prgrm, fn.GetInputs(prgrm), pkg, &inps)
		getFormattedParam(prgrm, fn.GetOutputs(prgrm), pkg, &outs)

		*ast1 += fmt.Sprintf("\t\t%d.- Function: %s (%s) (%s)\n",
			j, prgrm.CXProgramNameText(fn.Name), inps.String(), outs.String())

		for k, expr := range fn.Expressions {
			var inps bytes.Buffer
			var outs bytes.Buffer
			var opName1 string
			var lbl string

			// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
			// Now an arena getter, site S0480.
			cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
			if err != nil {
				panic(err)
			}

			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now an arena getter, site S0481.
			cxAtomicOpOperator := prgrm.GetFunctionFromArray(cxAtomicOp.Operator)
			// Adding label in case a `goto` statement was used for the expression.
			if cxAtomicOp.Label != stringpool.StringGidNil {
				lbl = " <<" + prgrm.CXProgramNameText(cxAtomicOp.Label) + ">>"
			} else {
				lbl = ""
			}

			// Determining operator's name.
			if cxAtomicOpOperator != nil {
				if cxAtomicOpOperator.IsBuiltIn() {

					opName1 = OpNames[cxAtomicOpOperator.AtomicOPCode]
				} else {
					opName1 = prgrm.CXProgramNameText(cxAtomicOpOperator.Name)
				}
			}

			getFormattedParam(prgrm, cxAtomicOp.GetInputs(prgrm), pkg, &inps)
			getFormattedParam(prgrm, cxAtomicOp.GetOutputs(prgrm), pkg, &outs)

			if expr.Type == CX_LINE {
				// OBJMODEL: state.CXLineMng.CXLineGet(gid), phase 5 #312.
				// Now an arena getter, site S0482.
				cxLine, _ := prgrm.GetCXLine(expr.Index)
				*ast1 += fmt.Sprintf("\t\t\t%d.- Line: %v: %s\n",
					k,
					cxLine.LineNumber,
					strings.TrimSpace(prgrm.CXProgramNameText(cxLine.LineStr)))
			} else if cxAtomicOpOperator != nil {
				assignOp := ""
				if outs.Len() > 0 {
					assignOp = " = "
				}
				*ast1 += fmt.Sprintf("\t\t\t%d.- Expression%s: %s%s%s(%s)\n",
					k,
					lbl,
					outs.String(),
					assignOp,
					opName1,
					inps.String(),
				)
			} else {
				// Then it's a variable declaration. These are represented
				// by expressions without operators that only have outputs.
				cxAtomicOpOutputs := cxAtomicOp.GetOutputs(prgrm)
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0483.
				cxAtomicOpOutputTypeSignature := prgrm.GetCXTypeSignatureFromArray(cxAtomicOpOutputs[len(cxAtomicOpOutputs)-1])
				if len(cxAtomicOpOutputs) > 0 {
					*ast1 += fmt.Sprintf("\t\t\t%d.- Declaration%s: %s %s\n",
						k,
						lbl,
						prgrm.CXProgramNameText(cxAtomicOpOutputTypeSignature.Name),
						GetFormattedType(prgrm, cxAtomicOpOutputTypeSignature))
				}
			}
		}

		j++
	}
}

// BuildStrPackages is an auxiliary function for `ToString`. It starts the
// process of building string format of the abstract syntax tree of a CX program.
func BuildStrPackages(prgrm *CXProgram, ast *string) {
	// We need to declare the counter outside so we can
	// ignore the increments from core or stdlib packages.
	var i int
	for _, pkgIdx := range packagesOrdered(prgrm) {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0484.
		pkg, err := prgrm.GetPackageFromArray(pkgIdx)
		if err != nil {
			panic(err)
		}

		if cxpackages.IsDefaultPackage(prgrm.CXProgramNameText(pkg.Name)) {
			continue
		}

		*ast += fmt.Sprintf("%d.- Package: %s\n", i, prgrm.CXProgramNameText(pkg.Name))

		buildStrImports(prgrm, pkg, ast)
		buildStrGlobals(prgrm, pkg, ast)
		buildStrStructs(prgrm, pkg, ast)
		buildStrFunctions(prgrm, pkg, ast)

		i++
	}
}

// packagesOrdered returns the packages of prgrm with each one after the
// packages it imports, in the order they were declared or imported, so -ast
// prints the same order on every run.
func packagesOrdered(prgrm *CXProgram) []CXPackageIndex {
	ordered := make([]CXPackageIndex, 0, len(prgrm.Packages))
	visited := map[CXPackageIndex]bool{}
	var visit func(pkgIdx CXPackageIndex)
	visit = func(pkgIdx CXPackageIndex) {
		if visited[pkgIdx] {
			return
		}
		visited[pkgIdx] = true
		if pkg, err := prgrm.GetPackageFromArray(pkgIdx); err == nil {
			for _, impIdx := range slices.Sorted(maps.Values(pkg.Imports)) {
				visit(impIdx)
			}
		}
		ordered = append(ordered, pkgIdx)
	}
	for _, pkgIdx := range slices.Sorted(maps.Values(prgrm.Packages)) {
		visit(pkgIdx)
	}
	return ordered
}

// getFormattedParam is an auxiliary function for `ToString`. It formats the
// name of a `CXExpression`'s input and output parameters (`CXArgument`s). Examples
// of these formattings are "pkg.foo[0]", "&*foo.field1". The result is written to
// `buf`.
func getFormattedParam(prgrm *CXProgram, paramTypeSigIdxs []CXTypeSignatureIndex, pkg *CXPackage, buf *bytes.Buffer) {
	for i, paramTypeSigIdx := range paramTypeSigIdxs {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0485.
		paramTypeSig := prgrm.GetCXTypeSignatureFromArray(paramTypeSigIdx)

		// Checking if this argument comes from an imported package.
		externalPkg := false
		if CXPackageIndex(pkg.Index) != paramTypeSig.Package {
			externalPkg = true
		}

		if paramTypeSig.Type == TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0486.
			param := prgrm.GetCXArgFromArray(CXArgumentIndex(paramTypeSig.Meta))

			buf.WriteString(fmt.Sprintf("%s %s", GetFormattedName(prgrm, param, externalPkg, pkg), GetFormattedType(prgrm, paramTypeSig)))
		} else {
			name := prgrm.CXProgramNameText(paramTypeSig.Name)

			// If it's a literal, just override the name with LITERAL_PLACEHOLDER.
			if paramTypeSig.Name == stringpool.StringGidNil {
				name = constants.LITERAL_PLACEHOLDER
			}

			// TODO: Check if external pkg and pkg name shown are correct
			if externalPkg {
				name = fmt.Sprintf("%s.%s", prgrm.CXProgramNameText(pkg.Name), name)
			}

			buf.WriteString(fmt.Sprintf("%s %s", name, GetFormattedType(prgrm, paramTypeSig)))
		}

		if i != len(paramTypeSigIdxs)-1 {
			buf.WriteString(", ")
		}

	}
}

// SignatureStringOfFunction returns the signature string of a function.
func SignatureStringOfFunction(prgrm *CXProgram, pkg *CXPackage, f *CXFunction) string {
	var ins bytes.Buffer
	var outs bytes.Buffer

	getFormattedParam(prgrm, f.GetInputs(prgrm), pkg, &ins)
	getFormattedParam(prgrm, f.GetOutputs(prgrm), pkg, &outs)

	return fmt.Sprintf("func %s(%s) (%s)",
		prgrm.CXProgramNameText(f.Name), ins.String(), outs.String())
}

// valueAt prints the value of type typ with dimensions dims at addr. A zero
// dimension is a slice, which holds a pointer to its object.
func valueAt(prgrm *CXProgram, addr types.Pointer, typ types.Code, size types.Pointer, structType *CXStruct, dims []types.Pointer, sep string) string {
	if len(dims) == 0 {
		if typ == types.POINTER {
			return fmt.Sprintf("%v", types.Read_ptr(prgrm.Memory, addr))
		}
		if structType != nil {
			return structAt(prgrm, addr, structType)
		}
		val, _ := readAtomic(prgrm.Memory, typ.Name(), addr)
		return val
	}

	stride := size
	for i := len(dims) - 1; i > 0; i-- {
		if dims[i] == 0 {
			stride = types.POINTER_SIZE
		} else {
			stride *= dims[i]
		}
	}

	if dims[0] != 0 {
		return elementsAt(prgrm, addr, dims[0], stride, typ, size, structType, dims[1:], sep)
	}
	sliceOffset := types.Read_ptr(prgrm.Memory, addr)
	return elementsAt(prgrm, sliceOffset+types.OBJECT_HEADER_SIZE+constants.SLICE_HEADER_SIZE, GetSliceLen(prgrm, sliceOffset), stride, typ, size, structType, dims[1:], sep)
}

// elementsAt prints the n elements of size stride that start at addr.
func elementsAt(prgrm *CXProgram, addr, n, stride types.Pointer, typ types.Code, size types.Pointer, structType *CXStruct, dims []types.Pointer, sep string) string {
	val := "["
	for c := types.Pointer(0); c < n; c++ {
		if c > 0 {
			val += sep
		}
		val += valueAt(prgrm, addr+c*stride, typ, size, structType, dims, sep)
	}
	return val + "]"
}

// structAt prints the fields of the struct instance at addr as {name: value, ...}.
func structAt(prgrm *CXProgram, addr types.Pointer, strct *CXStruct) string {
	val := "{"
	off := types.Pointer(0)
	for c, fldIdx := range strct.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0487.
		fld := prgrm.GetCXTypeSignatureFromArray(fldIdx)
		if c > 0 {
			val += ", "
		}
		val += prgrm.CXProgramNameText(fld.Name) + ": " + typeSignatureAt(prgrm, addr+off, fld)
		off += fld.GetSize(prgrm, false)
	}
	return val + "}"
}

// typeSignatureAt prints the value of the struct field typeSig at addr.
func typeSignatureAt(prgrm *CXProgram, addr types.Pointer, typeSig *CXTypeSignature) string {
	switch typeSig.Type {
	case TYPE_ATOMIC:
		val, _ := readAtomic(prgrm.Memory, types.Code(typeSig.Meta).Name(), addr)
		return val
	case TYPE_ARRAY_ATOMIC, TYPE_SLICE_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0488.
		arr := prgrm.GetCXTypeSignatureArrayFromArray(typeSig.Meta)
		eltType := types.Code(arr.Meta)
		return valueAt(prgrm, addr, eltType, eltType.Size(), nil, arr.Lengths, printSeparator(arr.Lengths))
	case TYPE_STRUCT:
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0490.
		return structAt(prgrm, addr, prgrm.GetCXTypeSignatureStructFromArray(typeSig.Meta).StructType)
	case TYPE_CXARGUMENT_DEPRECATE:
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0491.
		elt := prgrm.GetCXArgFromArray(CXArgumentIndex(typeSig.Meta))
		return valueAt(prgrm, addr, elt.Type, elt.Size, elt.StructType, elt.Lengths, printSeparator(elt.Lengths))
	}
	// Pointers print their address.
	return fmt.Sprintf("%v", types.Read_ptr(prgrm.Memory, addr))
}

// printSeparator separates the elements of a one dimensional collection with
// commas and those of a multi dimensional one with spaces.
func printSeparator(dims []types.Pointer) string {
	if len(dims) > 1 {
		return " "
	}
	return ", "
}

// readAtomic reads a value of the atomic type named typ at offset in memory.
// It returns false when typ is not an atomic type.
func readAtomic(memory []byte, typ string, offset types.Pointer) (string, bool) {
	switch typ {
	case "bool":
		return fmt.Sprintf("%v", types.Read_bool(memory, offset)), true
	case "str":
		return fmt.Sprintf("%v", types.Read_str(memory, offset)), true
	case "i8":
		return fmt.Sprintf("%v", types.Read_i8(memory, offset)), true
	case "i16":
		return fmt.Sprintf("%v", types.Read_i16(memory, offset)), true
	case "i32":
		return fmt.Sprintf("%v", types.Read_i32(memory, offset)), true
	case "i64":
		return fmt.Sprintf("%v", types.Read_i64(memory, offset)), true
	case "ui8":
		return fmt.Sprintf("%v", types.Read_ui8(memory, offset)), true
	case "ui16":
		return fmt.Sprintf("%v", types.Read_ui16(memory, offset)), true
	case "ui32":
		return fmt.Sprintf("%v", types.Read_ui32(memory, offset)), true
	case "ui64":
		return fmt.Sprintf("%v", types.Read_ui64(memory, offset)), true
	case "f32":
		return fmt.Sprintf("%v", types.Read_f32(memory, offset)), true
	case "f64":
		return fmt.Sprintf("%v", types.Read_f64(memory, offset)), true
	}
	return "", false
}

// argValueAt prints the value that elt, the last element of an argument's
// field path, names at addr after its indexes and dereferences.
func argValueAt(prgrm *CXProgram, addr types.Pointer, elt *CXArgument) string {
	typ := elt.Type
	if elt.IsPointer() {
		for _, op := range elt.DereferenceOperations {
			if op == constants.DEREF_POINTER {
				typ = elt.PointerTargetType
				break
			}
		}
	}
	dims := elt.Lengths
	if len(elt.Indexes) <= len(dims) {
		dims = dims[len(elt.Indexes):]
	}
	return valueAt(prgrm, addr, typ, elt.Size, elt.StructType, dims, printSeparator(dims))
}

// GetPrintableValue prints the value of argTypeSig in the frame at fp.
func GetPrintableValue(prgrm *CXProgram, fp types.Pointer, argTypeSig *CXTypeSignature) string {
	switch argTypeSig.Type {
	case TYPE_CXARGUMENT_DEPRECATE:
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0493.
		arg := prgrm.GetCXArgFromArray(CXArgumentIndex(argTypeSig.Meta))
		elt := arg.GetAssignmentElement(prgrm)
		return argValueAt(prgrm, GetFinalOffset(prgrm, fp, nil, argTypeSig), elt)
	case TYPE_ATOMIC:
		if argTypeSig.PassBy == constants.PASSBY_REFERENCE {
			return fmt.Sprintf("%v", GetFinalOffset(prgrm, fp, nil, argTypeSig))
		}
		val, _ := readAtomic(prgrm.Memory, types.Code(argTypeSig.Meta).Name(), GetFinalOffset(prgrm, fp, nil, argTypeSig))
		return val
	case TYPE_POINTER_ATOMIC:
		return fmt.Sprintf("%v", types.Read_ptr(prgrm.Memory, GetFinalOffset(prgrm, fp, nil, argTypeSig)))
	case TYPE_ARRAY_ATOMIC, TYPE_POINTER_ARRAY_ATOMIC, TYPE_SLICE_ATOMIC, TYPE_POINTER_SLICE_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0494.
		arr := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSig.Meta)
		eltType := types.Code(arr.Meta)
		dims := arr.Lengths
		if len(arr.Indexes) <= len(dims) {
			dims = dims[len(arr.Indexes):]
		}
		addr := GetFinalOffset(prgrm, fp, nil, argTypeSig)
		switch {
		case (argTypeSig.Type == TYPE_POINTER_ARRAY_ATOMIC || argTypeSig.Type == TYPE_POINTER_SLICE_ATOMIC) && argTypeSig.IsDeref:
			return valueAt(prgrm, addr, eltType, eltType.Size(), nil, dims, printSeparator(dims))
		case argTypeSig.Type == TYPE_POINTER_ARRAY_ATOMIC || argTypeSig.Type == TYPE_POINTER_SLICE_ATOMIC:
			return fmt.Sprintf("%v", types.Read_ptr(prgrm.Memory, addr))
		}
		return valueAt(prgrm, addr, eltType, eltType.Size(), nil, dims, printSeparator(dims))
	case TYPE_STRUCT, TYPE_POINTER_STRUCT:
		if argTypeSig.Type == TYPE_POINTER_STRUCT && !argTypeSig.IsDeref {
			return fmt.Sprintf("%v", types.Read_ptr(prgrm.Memory, GetFinalOffset(prgrm, fp, nil, argTypeSig)))
		}
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0496.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSig.Meta)
		addr := GetFinalOffset(prgrm, fp, nil, argTypeSig)
		if n := len(structDetails.Fields); n > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0497.
			return argValueAt(prgrm, addr, prgrm.GetCXArgFromArray(structDetails.Fields[n-1]))
		}
		return structAt(prgrm, addr, structDetails.StructType)
	}
	panic("type is not known")
}

// getFormattedDerefs is an auxiliary function for `GetFormattedName`. This
// function formats indexing and pointer dereferences associated to `arg`.
func getFormattedDerefs(prgrm *CXProgram, arg *CXArgument, includePkg bool, pkg *CXPackage) string {
	name := ""

	argPkg := pkg
	// Checking if we should include `arg`'s package name.
	if includePkg {
		name = fmt.Sprintf("%s.%s", prgrm.CXProgramNameText(argPkg.Name), prgrm.CXProgramNameText(arg.Name))
	} else {
		name = prgrm.CXProgramNameText(arg.Name)
	}

	// If it's a literal, just override the name with LITERAL_PLACEHOLDER.
	if arg.Name == stringpool.StringGidNil {
		name = constants.LITERAL_PLACEHOLDER
	}

	// Checking if we have indexing operations, e.g. foo[2][1]
	for _, idxIdx := range arg.Indexes {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0498.
		idx := prgrm.GetCXTypeSignatureFromArray(idxIdx)
		// Checking if the value is in data segment.
		// If this is the case, we can safely display it.
		idxValue := ""
		if idx.Offset > prgrm.Stack.Size {
			// Then it's a literal.
			idxI32 := types.Read_ptr(prgrm.Memory, idx.Offset)
			idxValue = fmt.Sprintf("%d", idxI32)
		} else {
			// Then let's just print the variable name.
			idxValue = prgrm.CXProgramNameText(idx.Name)
		}

		name = fmt.Sprintf("%s[%s]", name, idxValue)
	}

	return name
}

// GetFormattedName reads `arg.DereferenceOperations` and builds a string that
// depicts how an argument is being accessed. Example outputs: "foo[3]",
// "**bar", "foo.bar[0]". If `includePkg` is `true`, the argument name will
// include the package name that contains it, such as in "pkg.foo".
func GetFormattedName(prgrm *CXProgram, arg *CXArgument, includePkg bool, pkg *CXPackage) string {
	// Getting formatted name which does not include fields.
	name := getFormattedDerefs(prgrm, arg, includePkg, pkg)

	// Adding as suffixes all the fields.
	for _, fldIdx := range arg.Fields {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0499.
		fld := prgrm.GetCXArgFromArray(fldIdx)
		name = fmt.Sprintf("%s.%s", name, getFormattedDerefs(prgrm, fld, includePkg, pkg))
	}

	// Checking if we're referencing `arg`.
	if arg.PassBy == constants.PASSBY_REFERENCE {
		name = "&" + name
	}

	return name
}

// formatParameters returns a string containing a list of the formatted types of
// each of `params`, enclosed in parethesis. This function is used only when
// formatting functions as first-class objects.
func formatParameters(prgrm *CXProgram, params []CXTypeSignatureIndex) string {
	types := "("
	for i, paramIdx := range params {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0500.
		param := prgrm.GetCXTypeSignatureFromArray(paramIdx)
		types += GetFormattedType(prgrm, param)
		if i != len(params)-1 {
			types += ", "
		}
	}
	types += ")"

	return types
}

// GetFormattedType builds a string with the CXGO type representation of `arg`.
func GetFormattedType(prgrm *CXProgram, typeSig *CXTypeSignature) string {
	typ := ""

	if typeSig.Type == TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0501.
		expressionOutputArg := prgrm.GetCXArgFromArray(CXArgumentIndex(typeSig.Meta))
		typ = getFormattedType_CXArg(prgrm, expressionOutputArg)
	} else if typeSig.Type == TYPE_ATOMIC {
		typ = types.Code(typeSig.Meta).Name()

		if typeSig.PassBy == constants.PASSBY_REFERENCE {
			typ = "*" + typ
		}
	} else if typeSig.Type == TYPE_POINTER_ATOMIC {
		typ = types.Code(typeSig.Meta).Name()
		if !typeSig.IsDeref {
			typ = "*" + typ
		}
	} else if typeSig.Type == TYPE_ARRAY_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0502.
		arrayData := prgrm.GetCXTypeSignatureArrayFromArray(typeSig.Meta)

		arrLen := len(arrayData.Lengths) - len(arrayData.Indexes)
		if arrLen != 0 {
			dims := arrayData.Lengths[len(arrayData.Indexes):]
			for i := len(dims) - 1; i >= 0; i-- {
				typ = fmt.Sprintf("[%d]%s", dims[i], typ)
			}
		}

		typ += types.Code(arrayData.Meta).Name()
		if typeSig.PassBy == constants.PASSBY_REFERENCE {
			typ = "*" + typ
		}
	} else if typeSig.Type == TYPE_POINTER_ARRAY_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0503.
		arrayData := prgrm.GetCXTypeSignatureArrayFromArray(typeSig.Meta)

		arrLen := len(arrayData.Lengths) - len(arrayData.Indexes)
		if arrLen != 0 {
			dims := arrayData.Lengths[len(arrayData.Indexes):]
			for i := len(dims) - 1; i >= 0; i-- {
				typ = fmt.Sprintf("[%d]%s", dims[i], typ)
			}
		}

		typ += types.Code(arrayData.Meta).Name()
		if !typeSig.IsDeref {
			typ = "*" + typ
		}

	} else if typeSig.Type == TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0504.
		arrayData := prgrm.GetCXTypeSignatureArrayFromArray(typeSig.Meta)

		arrLen := len(arrayData.Lengths) - len(arrayData.Indexes)
		if arrLen != 0 {
			for i := 0; i < len(arrayData.Lengths[len(arrayData.Indexes):]); i++ {
				typ = fmt.Sprintf("[]%s", typ)
			}
		}

		typ += types.Code(arrayData.Meta).Name()
		if typeSig.PassBy == constants.PASSBY_REFERENCE {
			typ = "*" + typ
		}
	} else if typeSig.Type == TYPE_POINTER_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0505.
		arrayData := prgrm.GetCXTypeSignatureArrayFromArray(typeSig.Meta)

		arrLen := len(arrayData.Lengths) - len(arrayData.Indexes)
		if arrLen != 0 {
			for i := 0; i < len(arrayData.Lengths[len(arrayData.Indexes):]); i++ {
				typ = fmt.Sprintf("[]%s", typ)
			}
		}

		typ += types.Code(arrayData.Meta).Name()
		if !typeSig.IsDeref {
			typ = "*" + typ
		}
	} else if typeSig.Type == TYPE_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0506.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSig.Meta)
		lenFlds := len(structDetails.Fields)
		if lenFlds > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0507.
			fld := prgrm.GetCXArgFromArray(structDetails.Fields[lenFlds-1])
			fld = fld.GetAssignmentElement(prgrm)

			// TODO: to be replaced with correct
			// way of getting type, that is not
			// dependent on CXArg.
			typ = getFormattedType_CXArg(prgrm, fld)
		} else {
			typ = prgrm.CXProgramNameText(structDetails.StructType.Name)

			if typeSig.PassBy == constants.PASSBY_REFERENCE {
				typ = "*" + typ
			}
		}

	} else if typeSig.Type == TYPE_POINTER_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0508.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSig.Meta)
		lenFlds := len(structDetails.Fields)
		if lenFlds > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0509.
			fld := prgrm.GetCXArgFromArray(structDetails.Fields[lenFlds-1])
			fld = fld.GetAssignmentElement(prgrm)

			// TODO: to be replaced with correct
			// way of getting type, that is not
			// dependent on CXArg.
			typ = getFormattedType_CXArg(prgrm, fld)
		} else {
			typ = prgrm.CXProgramNameText(structDetails.StructType.Name)

			if !typeSig.IsDeref {
				typ = "*" + typ
			}
		}
	} else {
		panic("type is not known")
	}

	return typ
}

func getFormattedType_CXArg(prgrm *CXProgram, arg *CXArgument) string {
	typ := ""
	elt := arg.GetAssignmentElement(prgrm)

	// this is used to know what arg.Lengths index to use
	// used for cases like [5]*[3]i32, where we jump to another decl spec
	arrDeclCount := len(arg.Lengths) - 1
	// looping declaration specifiers
	for _, spec := range elt.DeclarationSpecifiers {
		switch spec {
		case constants.DECL_POINTER:
			typ = "*" + typ
		case constants.DECL_DEREF:
			typ = typ[1:]
		case constants.DECL_ARRAY:
			typ = fmt.Sprintf("[%d]%s", arg.Lengths[arrDeclCount], typ)
			arrDeclCount--
		case constants.DECL_SLICE:
			typ = "[]" + typ
		case constants.DECL_INDEXING:
		default:
			// base type
			if elt.StructType != nil {
				// then it's struct type
				typ += prgrm.CXProgramNameText(elt.StructType.Name)
			} else {
				if elt.Type == types.POINTER {
					typ += elt.PointerTargetType.Name()
				} else {
					// then it's basic type
					typ += elt.Type.Name()
				}

				// If it's a function, let's add the inputs and outputs.
				if elt.Type == types.FUNC {
					// if elt.IsLocalDeclaration {
					// Then it's a local variable, which can be assigned to a
					// lambda function, for example.
					// typ += formatParameters(prgrm, prgrm.ConvertIndexArgsToPointerArgs(elt.Inputs))
					// typ += formatParameters(prgrm, prgrm.ConvertIndexArgsToPointerArgs(elt.Outputs))
					// } else {
					// Then it refers to a named function defined in a package.
					// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
					// Now an arena getter, site S0510.
					pkg, err := prgrm.GetPackageFromArray(arg.Package)
					if err != nil {
						prgrm.CXProgramCompileErrorAddDebug(CompileErrorCodeMessage, elt.ArgDetails, CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})
						return typ
					}

					fn, err := pkg.GetFunction(prgrm, elt.Name)
					if err == nil {
						// Adding list of inputs and outputs types.
						typ += formatParameters(prgrm, fn.GetInputs(prgrm))
						typ += formatParameters(prgrm, fn.GetOutputs(prgrm))
					}
					// }
				}
			}
		}
	}
	return typ

}

// SignatureStringOfStruct returns the signature string of a struct.
func SignatureStringOfStruct(prgrm *CXProgram, s *CXStruct) string {
	fields := ""
	for _, typeSignatureIdx := range s.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0511.
		typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
		fields += fmt.Sprintf(" %s %s;", prgrm.CXProgramNameText(typeSignature.Name), GetFormattedType(prgrm, typeSignature))
	}

	return fmt.Sprintf("%s struct {%s }", prgrm.CXProgramNameText(s.Name), fields)
}

// GetArgSizeFromTypeName ...
func GetArgSizeFromTypeName(typeName string) int {
	switch typeName {
	case "bool", "i8", "ui8":
		return 1
	case "i16", "ui16":
		return 2
	case "str", "i32", "ui32", "f32", "aff":
		return 4
	case "i64", "ui64", "f64":
		return 8
	default:
		return 4
		// return -1
		// panic(CX_INTERNAL_ERROR)
	}
}
