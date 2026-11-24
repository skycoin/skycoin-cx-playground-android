package actions

import (
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// LiteralElement is one element of an array or slice literal. An elided
// element, `{1, 2}` or `{x: 1}`, takes its type from the enclosing literal.
type LiteralElement struct {
	Expressions []ast.CXExpression
	Elements    []LiteralElement
	Fields      []ast.CXExpression
	Elided      bool
	Keyed       bool
	Line        int
	Column      int
}

// LiteralType is the type of an array or slice literal. Lengths are
// outermost first and 0 for a slice dimension, as in CXArgument.Lengths.
type LiteralType struct {
	Lengths    []types.Pointer
	Slice      bool
	TypeCode   types.Code
	StructName string
	// Line and Column are where the literal starts, where a missing struct
	// is reported, or 0 for the token the parser is at.
	Line   int
	Column int
}

// LiteralElementsFlatten flattens elems of a literal of type typ into the
// expressions the literal actions take, building every elided element.
func LiteralElementsFlatten(prgrm *ast.CXProgram, typ LiteralType, elems []LiteralElement) []ast.CXExpression {
	var result []ast.CXExpression
	for _, elem := range elems {
		exprs := elem.Expressions
		if elem.Elided {
			exprs = literalElementElidedBuild(prgrm, LiteralType{Lengths: typ.Lengths[1:], Slice: typ.Slice, TypeCode: typ.TypeCode, StructName: typ.StructName}, elem)
			if exprs == nil {
				// The element is a compile error, already recorded.
				continue
			}
		}
		// Only a slice or struct array literal reads the position, from the
		// line it adds.
		sliceColumn := 0
		if typ.Slice || typ.StructName != "" {
			sliceColumn = elem.Column
		}
		result = append(result, SliceLiteralElement(prgrm, exprs, elem.Line, sliceColumn)...)
	}
	return result
}

// literalElementElidedBuild builds the elided element elem as a literal of type typ,
// so `[][]i32{{1}}` is `[][]i32{[]i32{1}}` and `[]P{{x: 1}}` is `[]P{P{x: 1}}`.
func literalElementElidedBuild(prgrm *ast.CXProgram, typ LiteralType, elem LiteralElement) []ast.CXExpression {
	if len(typ.Lengths) == 0 {
		if typ.StructName == "" {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeLiteralElementType, CurrentFile, elem.Line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(typ.TypeCode.Name())})
			return nil
		}
		if !elem.Keyed {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeStructLiteralFieldNames, CurrentFile, elem.Line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(typ.StructName)})
			return nil
		}
		return PrimaryStructLiteral(prgrm, typ.StructName, elem.Line, 0, elem.Fields)
	}
	if elem.Keyed && len(elem.Fields) > 0 {
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeLiteralElementType, CurrentFile, elem.Line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(literalTypeName(typ))})
		return nil
	}
	inner := LiteralElementsFlatten(prgrm, typ, elem.Elements)
	if typ.Slice {
		return LiteralSlice(prgrm, typ, inner)
	}
	if typ.StructName != "" {
		return ArrayLiteralStructExpression(prgrm, typ.Lengths, typ.StructName, elem.Line, 0, inner)
	}
	return ArrayLiteralExpression(prgrm, typ.Lengths, typ.TypeCode, elem.Line, 0, inner)
}

// LiteralSlice builds a slice literal of type typ from flattened elements,
// adding one slice dimension for each length past the first.
func LiteralSlice(prgrm *ast.CXProgram, typ LiteralType, exprs []ast.CXExpression) []ast.CXExpression {
	var result []ast.CXExpression
	if typ.StructName != "" {
		result = SliceLiteralStructExpression(prgrm, typ.StructName, typ.Line, typ.Column, exprs)
		if literalValueInvalid(prgrm, result) {
			// The struct does not exist, and its error is recorded.
			return result
		}
	} else {
		result = SliceLiteralExpression(prgrm, typ.TypeCode, exprs)
	}
	if len(result) == 0 {
		return nil
	}
	for range typ.Lengths[1:] {
		result = LiteralSliceNest(prgrm, result)
	}
	return result
}

// literalTypeName is the name of typ as written in CX, such as [2][]i32.
func literalTypeName(typ LiteralType) string {
	name := typ.StructName
	if name == "" {
		name = typ.TypeCode.Name()
	}
	prefix := ""
	for _, length := range typ.Lengths {
		if typ.Slice {
			prefix += "[]"
		} else {
			prefix += "[" + strconv.FormatUint(uint64(length), 10) + "]"
		}
	}
	return prefix + name
}

// LiteralSliceNest makes the slice literal exprs one dimension deeper, so
// `[]` before `[]i32{[]i32{1}}` gives a [][]i32.
func LiteralSliceNest(prgrm *ast.CXProgram, exprs []ast.CXExpression) []ast.CXExpression {
	lastExpression, err := prgrm.GetCXAtomicOpFromExpressions(exprs, len(exprs)-1)
	if err != nil {
		panic(err)
	}
	for _, expr := range exprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S1486.
		expression, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now arena getters, sites S1487 S1488.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		lastExpressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastExpression.GetInputs(prgrm)[0])
		switch expressionOutputTypeSig.Type {
		case ast.TYPE_CXARGUMENT_DEPRECATE:
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1489.
			expressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
			if expressionOutputArg.Name == lastExpressionInputTypeSig.Name {
				expressionOutputArg.Lengths = append(expressionOutputArg.Lengths, 0)
				expressionOutputArg.DeclarationSpecifiers = append(expressionOutputArg.DeclarationSpecifiers, constants.DECL_SLICE)
			}
		case ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
			if expressionOutputTypeSig.Name == lastExpressionInputTypeSig.Name {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, sites S1490 S1491.
				sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionOutputTypeSig.Meta)
				sliceDetails.Lengths = append(sliceDetails.Lengths, 0)
			}
		}
	}
	// The value the literal gives, read by := and calls, gets the dimension too.
	if outputs := lastExpression.GetOutputs(prgrm); len(outputs) > 0 {
		if sig := prgrm.GetCXTypeSignatureFromArray(outputs[0]); sig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			out := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta))
			out.Lengths = append(out.Lengths, 0)
			out.DeclarationSpecifiers = append(out.DeclarationSpecifiers, constants.DECL_SLICE)
		} else if sig.Type == ast.TYPE_SLICE_ATOMIC || sig.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
			details := prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta)
			details.Lengths = append(details.Lengths, 0)
		}
	}
	exprs[len(exprs)-1].ExpressionType = ast.CXEXPR_ARRAY_LITERAL
	return exprs
}

// literalValueInvalid reports whether the value exprs is the invalid value
// of structLiteralInvalid.
func literalValueInvalid(prgrm *ast.CXProgram, exprs []ast.CXExpression) bool {
	if len(exprs) == 0 || exprs[len(exprs)-1].Type == ast.CX_LINE {
		return false
	}
	outputs := prgrm.CXAtomicOps[exprs[len(exprs)-1].Index].GetOutputs(prgrm)
	return len(outputs) == 1 && typeSignatureInvalid(prgrm, prgrm.GetCXTypeSignatureFromArray(outputs[0]))
}

// LiteralTypeSlices makes elem the element type of count slice dimensions,
// as [][] before *T gives [][]*T.
func LiteralTypeSlices(elem *ast.CXArgument, count int) *ast.CXArgument {
	for i := 0; i < count; i++ {
		elem = DeclarationSpecifiers(elem, []types.Pointer{0}, constants.DECL_SLICE)
	}
	return elem
}

// LiteralCompound builds a literal of the array or slice type typ, whose
// elements are pointers or have array and slice dimensions mixed, as
// []*T{&t} and [2][]i32{{1}, {2, 3}} do. line and column are where it starts.
func LiteralCompound(prgrm *ast.CXProgram, typ *ast.CXArgument, line int, column int, elems []LiteralElement) []ast.CXExpression {
	specs := typ.DeclarationSpecifiers
	slice := specs[len(specs)-1] == constants.DECL_SLICE
	elem := literalTemplateElement(prgrm, typ)
	var exprs []ast.CXExpression
	for _, element := range elems {
		element := element
		value := element.Expressions
		if element.Elided {
			if value = literalCompoundElided(prgrm, elem, element); value == nil {
				// The element is a compile error, already recorded.
				continue
			}
		}
		elementColumn := 0
		if slice {
			elementColumn = element.Column
		}
		exprs = append(exprs, SliceLiteralElement(prgrm, value, element.Line, elementColumn)...)
	}
	if slice {
		return sliceLiteralExpressionOf(prgrm, elem.Type, elem.StructType, elem, exprs)
	}
	return arrayLiteralExpressionOf(prgrm, typ.Lengths[:1], elem.Type, elem.StructType, elem, line, column, exprs)
}

// literalTemplateLengths returns the lengths of an array literal of
// arraySizes whose elements have the type template, nil for a basic or
// struct element.
func literalTemplateLengths(arraySizes []types.Pointer, template *ast.CXArgument) []types.Pointer {
	if template == nil {
		return arraySizes
	}
	return append(append([]types.Pointer(nil), arraySizes...), template.Lengths...)
}

// literalTemplateElement returns a new argument of the element type of the
// array or slice type typ.
func literalTemplateElement(prgrm *ast.CXProgram, typ *ast.CXArgument) *ast.CXArgument {
	elem := argumentCopy(prgrm, typ)
	elem.DeclarationSpecifiers = elem.DeclarationSpecifiers[:len(elem.DeclarationSpecifiers)-1]
	elem.Lengths = elem.Lengths[1:]
	return elem
}

// literalCompoundElided builds the elided element elem of a literal whose
// elements have the type typ. As Go, {x: 1} for a *T is &T{x: 1}.
func literalCompoundElided(prgrm *ast.CXProgram, typ *ast.CXArgument, elem LiteralElement) []ast.CXExpression {
	if simple, ok := literalTypeSimple(prgrm, typ); ok {
		return literalElementElidedBuild(prgrm, simple, elem)
	}
	specs := typ.DeclarationSpecifiers
	switch last := specs[len(specs)-1]; {
	case last == constants.DECL_ARRAY || last == constants.DECL_SLICE:
		if !elem.Keyed || len(elem.Fields) == 0 {
			return LiteralCompound(prgrm, typ, elem.Line, 0, elem.Elements)
		}
	case last == constants.DECL_POINTER && len(specs) == 2 && specs[0] == constants.DECL_STRUCT && literalStructLocal(prgrm, typ.StructType):
		if !elem.Keyed {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeStructLiteralFieldNames, CurrentFile, elem.Line, ast.CompileErrorOperands{int64(typ.StructType.Name)})
			return nil
		}
		return UnaryExpression(prgrm, "&", PrimaryStructLiteral(prgrm, prgrm.CXProgramNameText(typ.StructType.Name), elem.Line, 0, elem.Fields))
	}
	prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeLiteralElementType, CurrentFile, elem.Line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(ast.GetFormattedType(prgrm, prgrm.GetCXTypeSignatureFromArray(ast.AllocCXTypeSignatureOfCXArg(prgrm, typ))))})
	return nil
}

// literalTypeSimple returns typ as a LiteralType when it is a basic or
// struct type of the current package with only array or only slice
// dimensions, the types the literal actions build without a template.
func literalTypeSimple(prgrm *ast.CXProgram, typ *ast.CXArgument) (LiteralType, bool) {
	specs := typ.DeclarationSpecifiers
	if len(specs) == 0 || typ.Type == types.POINTER {
		return LiteralType{}, false
	}
	result := LiteralType{Lengths: append([]types.Pointer(nil), typ.Lengths...), TypeCode: typ.Type}
	switch specs[0] {
	case constants.DECL_BASIC:
	case constants.DECL_STRUCT:
		if !literalStructLocal(prgrm, typ.StructType) {
			return LiteralType{}, false
		}
		result.TypeCode = 0
		result.StructName = prgrm.CXProgramNameText(typ.StructType.Name)
	default:
		return LiteralType{}, false
	}
	for i, spec := range specs[1:] {
		if spec != constants.DECL_ARRAY && spec != constants.DECL_SLICE || i > 0 && spec != specs[1] {
			return LiteralType{}, false
		}
		result.Slice = spec == constants.DECL_SLICE
	}
	return result, true
}

// literalStructLocal reports whether strct is a struct of the current package.
func literalStructLocal(prgrm *ast.CXProgram, strct *ast.CXStruct) bool {
	pkg, err := prgrm.GetCurrentPackage()
	return err == nil && strct != nil && strct.Package == ast.CXPackageIndex(pkg.Index)
}

// literalTypeCompound reports whether typ is a type only LiteralCompound
// builds, one with a pointer or with array and slice dimensions mixed.
func literalTypeCompound(typ *ast.CXArgument) bool {
	for _, spec := range typ.DeclarationSpecifiers {
		if spec == constants.DECL_POINTER {
			return true
		}
	}
	return typ.ArraySliceMixed()
}
