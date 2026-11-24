package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

func processGlobalInitializer(prgrm *ast.CXProgram, initializer []ast.CXExpression, glblIdx ast.CXTypeSignatureIndex) {
	start := len(prgrm.SysInitExprs)
	glbl := prgrm.GetCXTypeSignatureFromArray(glblIdx)
	defer prgrm.CXProgramGlobalInitializerAdd(prgrm.CurrentPackage, glbl.Name, start)
	initializerExpressionIdx := initializer[len(initializer)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1209.
	initializerExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[initializerExpressionIdx].Operator)
	// then we just re-assign offsets
	if initializerExpressionOperator == nil {
		// then it's a literal
		typeSigIdx := prgrm.CXAtomicOps[initializerExpressionIdx].GetOutputs(prgrm)[0]
		prgrm.CXAtomicOps[initializerExpressionIdx].AddInput(prgrm, typeSigIdx)
		prgrm.CXAtomicOps[initializerExpressionIdx].Outputs = nil

		prgrm.CXAtomicOps[initializerExpressionIdx].AddOutput(prgrm, glblIdx)
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now an arena insert, site S1210.
		opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_IDENTITY])
		prgrm.CXAtomicOps[initializerExpressionIdx].Operator = opIdx

		//add intialization statements, to array
		prgrm.SysInitExprs = append(prgrm.SysInitExprs, initializer...)
	} else {
		// then it's an expression
		if initializer[len(initializer)-1].IsStructLiteral() {
			// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
			// Now a heap literal, site S1211.
			outputStruct := &ast.CXStruct{}
			outputStruct.AddField_TypeSignature(prgrm, glblIdx)
			// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
			// Now a heap literal, site S1212 S1213.
			index := prgrm.AddCXAtomicOp(&ast.CXAtomicOperator{Outputs: outputStruct, Operator: -1, Function: -1})
			initializer = StructLiteralAssignment(prgrm,
				// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
				// Now a slice of objects, site S1214.
				[]ast.CXExpression{
					{
						Index: index,
						Type:  ast.CX_ATOMIC_OPERATOR,
					},
				},
				initializer,
			)
		} else {
			prgrm.CXAtomicOps[initializerExpressionIdx].Outputs = nil
			prgrm.CXAtomicOps[initializerExpressionIdx].AddOutput(prgrm, glblIdx)
		}

		//add intialization statements, to array
		prgrm.SysInitExprs = append(prgrm.SysInitExprs, initializer...)
	}
}

// DeclareGlobalInPackage creates a global variable in a specified package
//
// If `doesInitialize` is true, then `initializer` is used to initialize the
// new variable.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	pkg - the package where the global will belong.
//	declarator - contains the name of the global var.
//	declaration_specifiers - contains the type build of the global.
//	initializer - if doesInitialize is true then this contains the initial
//				  value of the global.
//	doesInitialize - true if global is initialized or given a value.
func DeclareGlobalInPackage(prgrm *ast.CXProgram, pkg *ast.CXPackage,
	declarator *ast.CXArgument, declaration_specifiers *ast.CXArgument,
	initializer []ast.CXExpression, doesInitialize bool) {
	if pkg == nil {
		var err error
		pkg, err = prgrm.GetCurrentPackage()
		if err != nil {
			panic(err)
		}
	}
	declaration_specifiers.Package = ast.CXPackageIndex(pkg.Index)

	totalSize := declaration_specifiers.Size
	for _, decl := range declaration_specifiers.DeclarationSpecifiers {
		switch decl {
		case constants.DECL_POINTER, constants.DECL_SLICE:
			totalSize = types.POINTER_SIZE
		case constants.DECL_ARRAY:
			totalSize = declaration_specifiers.ArraySize()
		}
	}
	if size, fits := declarationArgSize(declaration_specifiers); !fits || !declarationSizeSumFits(prgrm.Data.StartsAt+prgrm.Data.Size, size) {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeDeclarationTooLarge, declarator.ArgDetails, ast.CompileErrorOperands{int64(declarator.Name)})
		totalSize = 0
	}

	// ParseGlobals passes the argument the global already names as declarator.
	declaratorTaken := false
	if _, err := pkg.GetGlobal(prgrm, declarator.Name); err != nil {
		globalPlaceholderAdd(prgrm, pkg, declarator)
	}
	// Treat the name a bit different whether it's defined already or not.
	if glbl, err := pkg.GetGlobal(prgrm, declarator.Name); err == nil {
		// then it was only added a reference to the symbol

		// if offset is not valid, assign new offset
		// The offset and pass-by of the new data, zero when the offset is valid.
		var glblArgOffset types.Pointer
		var glblArgPassBy int
		if !glbl.Offset.IsValid() {
			var glblArg *ast.CXArgument
			if declaration_specifiers.IsSlice() { // TODO:PTR move branch in WritePrimary
				glblArg = WritePrimary(prgrm, declaration_specifiers.Type,
					make([]byte, types.POINTER_SIZE), true)
			} else {
				glblArg = WritePrimary(prgrm, declaration_specifiers.Type,
					make([]byte, totalSize), true)
			}
			glblArgOffset = glblArg.Offset
			glblArgPassBy = glblArg.PassBy
			prgrm.ReleaseCXArgBuilder(glblArg)
		}

		if ast.IsTypeAtomic(declaration_specifiers) {
			glbl.Type = ast.TYPE_ATOMIC
			glbl.Meta = int(declaration_specifiers.Type)
			if !glbl.Offset.IsValid() {
				glbl.Offset = glblArgOffset
			}

			glbl.PassBy = declaration_specifiers.PassBy
		} else if ast.IsTypePointerAtomic(declaration_specifiers) {
			glbl.Type = ast.TYPE_POINTER_ATOMIC
			glbl.Meta = int(declaration_specifiers.Type)
			if !glbl.Offset.IsValid() {
				glbl.Offset = glblArgOffset
			}

			glbl.PassBy = declaration_specifiers.PassBy
		} else if ast.IsTypeArrayAtomic(declaration_specifiers) {
			glbl.Type = ast.TYPE_ARRAY_ATOMIC

			arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
			typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
			typeSignatureForArray.Meta = int(declaration_specifiers.Type)
			typeSignatureForArray.Lengths = declaration_specifiers.Lengths
			typeSignatureForArray.Indexes = declaration_specifiers.Indexes

			glbl.Meta = int(objmodel.GidIndex(int64(arrayGid)))

			glbl.PassBy = declaration_specifiers.PassBy

			if !glbl.Offset.IsValid() {
				glbl.Offset = glblArgOffset
			}
		} else if ast.IsTypePointerArrayAtomic(declaration_specifiers) {
			glbl.Type = ast.TYPE_POINTER_ARRAY_ATOMIC

			arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
			typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
			typeSignatureForArray.Meta = int(declaration_specifiers.Type)
			typeSignatureForArray.Lengths = declaration_specifiers.Lengths
			typeSignatureForArray.Indexes = declaration_specifiers.Indexes
			glbl.Meta = int(objmodel.GidIndex(int64(arrayGid)))
			glbl.PassBy = declaration_specifiers.PassBy

			if !glbl.Offset.IsValid() {
				glbl.Offset = glblArgOffset
			}
		} else if ast.IsTypeSliceAtomic(declaration_specifiers) {
			// If slice atomic type, i.e. []i32, []f64, etc.

			glbl.Type = ast.TYPE_SLICE_ATOMIC

			arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
			typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
			typeSignatureForArray.Meta = int(declaration_specifiers.Type)
			typeSignatureForArray.Lengths = declaration_specifiers.Lengths
			typeSignatureForArray.Indexes = declaration_specifiers.Indexes
			glbl.Meta = int(objmodel.GidIndex(int64(arrayGid)))
			glbl.PassBy = declaration_specifiers.PassBy

			if !glbl.Offset.IsValid() {
				glbl.Offset = glblArgOffset
			}
		} else if ast.IsTypePointerSliceAtomic(declaration_specifiers) {
			// If pointer slice atomic type, i.e. *[]i32, *[]f64, etc.

			glbl.Type = ast.TYPE_POINTER_SLICE_ATOMIC

			arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
			typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
			typeSignatureForArray.Meta = int(declaration_specifiers.Type)
			typeSignatureForArray.Lengths = declaration_specifiers.Lengths
			typeSignatureForArray.Indexes = declaration_specifiers.Indexes
			glbl.Meta = int(objmodel.GidIndex(int64(arrayGid)))
			glbl.PassBy = declaration_specifiers.PassBy

			if !glbl.Offset.IsValid() {
				glbl.Offset = glblArgOffset
			}
		} else {
			// its a cxargument_deprecate type
			var glblIdx int
			var glblCXArg *ast.CXArgument
			if glbl.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				glblIdx = glbl.Meta
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1225.
				glblCXArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(glblIdx))
			} else {
				panic("type is not type cx argument deprecate\n\n")
			}

			if glblCXArg.Size == 0 || totalSize == 0 {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1763.
				prgrm.GetCXArg(ast.CXArgumentIndex(glblIdx)).Offset = glblArgOffset
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1764.
				prgrm.GetCXArg(ast.CXArgumentIndex(glblIdx)).PassBy = glblArgPassBy
			}

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1765.
			declaration_specifiers.Name = prgrm.GetCXArg(ast.CXArgumentIndex(glblIdx)).Name
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1766.
			declaration_specifiers.Offset = prgrm.GetCXArg(ast.CXArgumentIndex(glblIdx)).Offset
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1767.
			declaration_specifiers.PassBy = prgrm.GetCXArg(ast.CXArgumentIndex(glblIdx)).PassBy
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1768.
			declaration_specifiers.Package = prgrm.GetCXArg(ast.CXArgumentIndex(glblIdx)).Package

			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now an object copy, site S1226. Also CXArgumentGet, phase 5 #327, site S1770.
			*prgrm.GetCXArg(ast.CXArgumentIndex(glblIdx)) = *declaration_specifiers
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1771.
			prgrm.GetCXArg(ast.CXArgumentIndex(glblIdx)).Index = glblIdx

		}

		declaratorTaken = glbl.Type == ast.TYPE_CXARGUMENT_DEPRECATE && glbl.Meta == declarator.Index

		// The global took a copy of the declaration, and of its debug record when
		// it names an argument.
		if glbl.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			prgrm.ReleaseCXArgUnlessNamed(declaration_specifiers, glbl.Index)
		} else {
			prgrm.ReleaseCXArgBuilder(declaration_specifiers)
		}

		// Checking if something is supposed to be initialized
		// and if `initializer` actually contains something.
		// If `initializer` is `nil`, this means that an expression
		// equivalent to nil was used, such as `[]i32{}`.
		if doesInitialize && initializer != nil {
			processGlobalInitializer(prgrm, initializer, glbl.Index)
		}
	} else {
		// then it hasn't been defined
		var glblArg *ast.CXArgument
		if declaration_specifiers.IsSlice() { // TODO:PTR move branch in WritePrimary
			glblArg = WritePrimary(prgrm, declaration_specifiers.Type, make([]byte, types.POINTER_SIZE), true)
		} else {
			glblArg = WritePrimary(prgrm, declaration_specifiers.Type, make([]byte, totalSize), true)
		}

		declaration_specifiers.Name = declarator.Name
		// The second name of var a, b T is declared first here, at its own position.
		declaratorFile, declaratorLine, declaratorColumn := ast.CXArgumentDebugPosition(prgrm, declarator.ArgDetails)
		declaratorDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(declaration_specifiers.ArgDetails)
		declaratorDetails.FileName, declaratorDetails.FileLine, declaratorDetails.FileColumn = declaratorFile, declaratorLine, declaratorColumn
		declaration_specifiers.Offset = glblArg.Offset
		declaration_specifiers.Size = glblArg.Size
		prgrm.ReleaseCXArgBuilder(glblArg)
		declaration_specifiers.Package = ast.CXPackageIndex(pkg.Index)

		typeSignatureIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, declaration_specifiers)
		pkg.AddGlobal_TypeSignature(prgrm, typeSignatureIdx)

		// Checking if something is supposed to be initialized
		// and if `initializer` actually contains something.
		// If `initializer` is `nil`, this means that an expression
		// equivalent to nil was used, such as `[]i32{}`.
		if doesInitialize && initializer != nil {
			processGlobalInitializer(prgrm, initializer, typeSignatureIdx)
		}
	}

	// Otherwise only the name and line of the declarator are used.
	if !declaratorTaken {
		prgrm.ReleaseCXArgBuilder(declarator)
	}
}

func processLocalInitialization(prgrm *ast.CXProgram, pkg *ast.CXPackage, initializer *[]ast.CXExpression, localVarIdx ast.CXTypeSignatureIndex, declSpecIdx ast.CXArgumentIndex) {
	initializerExpression, err := prgrm.GetCXAtomicOpFromExpressions(*initializer, len(*initializer)-1)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1229.
	initializerExpressionOperator := prgrm.GetFunctionFromArray(initializerExpression.Operator)
	if initializerExpressionOperator == nil {
		// THEN it's a literal, e.g. var foo i32 = 10;

		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S1230.
		exprCXLine := MakeCXLineExpressionCurrent(prgrm)

		// we need to create an expression that links the initializer expressions
		// with the declared variable
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S1231.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
		cxExprAtomicOpIdx := expr.Index
		prgrm.CXAtomicOps[cxExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)

		var initOutTypeSigIdx ast.CXTypeSignatureIndex
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1232.
		initializerExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(initializerExpression.GetOutputs(prgrm)[0])
		if initializerExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1233.
			initializerExpressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(initializerExpressionOutputTypeSig.Meta))
			initOut := initializerExpressionOutputArg
			initOutIdx := ast.CXArgumentIndex(initializerExpressionOutputTypeSig.Meta)

			// CX checks the output of an expression to determine if it's being passed
			// by value or by reference, so we copy this property from the initializer's
			// output, in case of something like var foo *i32 = &bar
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1772.
			prgrm.GetCXArg(ast.CXArgumentIndex(declSpecIdx)).PassBy = initOut.PassBy

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1234.
			initOutTypeSigIdx = ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArgFromArray(initOutIdx))
		} else if initializerExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || initializerExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			initOutTypeSigIdx = initializerExpression.GetOutputs(prgrm)[0]
		} else if initializerExpressionOutputTypeSig.Type == ast.TYPE_STRUCT {
			initOutTypeSigIdx = initializerExpression.GetOutputs(prgrm)[0]
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[cxExprAtomicOpIdx].AddOutput(prgrm, localVarIdx)
		prgrm.CXAtomicOps[cxExprAtomicOpIdx].AddInput(prgrm, initOutTypeSigIdx)

		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now an object copy, site S1236.
		(*initializer)[len(*initializer)-1] = *exprCXLine
		(*initializer) = append(*initializer, *expr)

	} else {
		// ELSE it's an expression with an operator

		expr := (*initializer)[len(*initializer)-1]
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S1237.
		cxExprAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		// THEN the expression has outputs created from the result of
		// handling a dot notation initializer, and it needs to be replaced
		// ELSE we simply add it using `AddOutput`
		if len(cxExprAtomicOp.GetOutputs(prgrm)) > 0 {
			// declSpecIdx := prgrm.AddCXArgInArray(declarationSpecifiers)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
			// Now a dropped object, site S1238.
			cxExprAtomicOp.Outputs.Fields = nil

		}
		cxExprAtomicOp.AddOutput(prgrm, localVarIdx)
	}
}

// DeclareLocal() creates a local variable inside a function.
//
// Returns a list of expressions that contains the initialization, if any.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	declarator - contains the name of the var.
//	declaration_specifiers - contains the type build of the var.
//	initializer - if doesInitialize is true then this contains the initial
//				  value of the var.
//	doesInitialize - true if var is initialized or given a value.
func DeclareLocal(prgrm *ast.CXProgram, declarator *ast.CXArgument, declarationSpecifiers *ast.CXArgument,
	initializer []ast.CXExpression, doesInitialize bool) []ast.CXExpression {
	if _, fits := declarationArgSize(declarationSpecifiers); !fits {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeDeclarationTooLarge, declarator.ArgDetails, ast.CompileErrorOperands{int64(declarator.Name)})
	}

	currFn, err := prgrm.GetCurrentFunction()
	if err != nil {
		// TODO: improve error handling
		panic("error getting current function")
	}
	err = currFn.AddLocalVariableName(declarator.Name)
	if err != nil {
		panic("error adding local variable")
	}

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1239.
	declCXLine := MakeCXLineExpressionCurrent(prgrm)
	// Declaration expression to handle the inline initialization.
	// For example, `var foo i32 = 11` needs to be divided into two expressions:
	// one that declares `foo`, and another that assigns 11 to `foo`
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1240.
	decl := ast.MakeAtomicOperatorExpression(prgrm, nil)
	expressionIdx := decl.Index
	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	declarationSpecifiers.Name = declarator.Name
	_, declaratorLine, declaratorColumn := ast.CXArgumentDebugPosition(prgrm, declarator.ArgDetails)
	declaratorDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(declarationSpecifiers.ArgDetails)
	declaratorDetails.FileLine, declaratorDetails.FileColumn = declaratorLine, declaratorColumn
	declarationSpecifiers.Package = ast.CXPackageIndex(pkg.Index)
	declarationSpecifiers.PreviouslyDeclared = true
	declSpecIdx := ast.CXArgumentIndex(declarationSpecifiers.Index)

	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S1242.
	localVarTypeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArgFromArray(declSpecIdx))
	prgrm.CXAtomicOps[expressionIdx].AddOutput(prgrm, localVarTypeSigIdx)
	// Only the name and line of the declarator are used.
	prgrm.ReleaseCXArgBuilder(declarator)

	// Checking if something is supposed to be initialized
	// and if `initializer` actually contains something.
	// If `initializer` is `nil`, this means that an expression
	// equivalent to nil was used, such as `[]i32{}`.
	if doesInitialize && initializer != nil {
		outputlessValueCheck(prgrm, initializer)
		processLocalInitialization(prgrm, pkg, &initializer, localVarTypeSigIdx, declSpecIdx)
		prgrm.ReleaseCXArgUnlessNamed(declarationSpecifiers, localVarTypeSigIdx)

		// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
		// Now a slice of objects, site S1244.
		return append([]ast.CXExpression{*declCXLine, *decl}, initializer...)
	} else {
		// There is no initialization.
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S1245.
		expr := ast.MakeAtomicOperatorExpression(prgrm, nil)
		cxAtomicOpIdx := expr.Index
		prgrm.CXAtomicOps[cxAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)
		prgrm.CXAtomicOps[cxAtomicOpIdx].AddOutput(prgrm, localVarTypeSigIdx)
		prgrm.ReleaseCXArgUnlessNamed(declarationSpecifiers, localVarTypeSigIdx)

		// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
		// Now a slice of objects, site S1246.
		return []ast.CXExpression{*declCXLine, *expr}
	}

}

// GlobalRedeclarationCheck reports whether declarator declares a global of
// the current package a second time, and records the error. The first
// declaration gives the global the position of its name, which pass two
// meets again. A global without a debug record is declared once it has an
// offset.
func GlobalRedeclarationCheck(prgrm *ast.CXProgram, declarator *ast.CXArgument) bool {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		return false
	}
	glbl, err := pkg.GetGlobal(prgrm, declarator.Name)
	if err != nil {
		return false
	}
	file, line, column := ast.CXArgumentDebugPosition(prgrm, declarator.ArgDetails)
	firstFile, firstLine, firstColumn := ast.CXArgumentDebugPosition(prgrm, glbl.ArgDetails)
	if glbl.ArgDetails == ast.CXArgumentDebugGidNil {
		if !glbl.Offset.IsValid() {
			return false
		}
	} else if firstLine == 0 {
		details := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(glbl.ArgDetails)
		details.FileName, details.FileLine, details.FileColumn = file, line, column
		return false
	} else if firstFile == file && firstLine == line && firstColumn == column {
		return false
	}
	prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeRedeclaredBlock, file, line, column, ast.CompileErrorOperands{int64(declarator.Name), int64(firstFile), int64(firstLine), int64(firstColumn)})
	return true
}

// GlobalValue is a value of var a, b = x, y as pass one reads it, the type
// of a literal or types.UNDEFINED, and the positions of its start and of the
// token after it.
type GlobalValue struct {
	GlobalValueType        types.Code
	GlobalValueStartLine   int
	GlobalValueStartColumn int
	GlobalValueEndLine     int
	GlobalValueEndColumn   int
}

// DeclareGlobals declares the globals of var a, b T and var a, b T = x, y.
// Pass one passes each value in passOneValues, and pass two the values. A
// spec without a type, var a, b = x, y, takes the type of each literal value
// in pass one, and the inference pass of t9-cxparsing gives the others the
// type of their value. Pass two then only adds the initializer.
func DeclareGlobals(prgrm *ast.CXProgram, names []*ast.CXArgument, spec *ast.CXArgument, values [][]ast.CXExpression, passOneValues []GlobalValue, initialized bool) {
	count := len(values)
	if passOneValues != nil {
		count = len(passOneValues)
	}
	// A literal is one value, which only a call of several results is not.
	if initialized && count == 1 && len(names) > 1 && (passOneValues == nil || spec != nil || passOneValues[0].GlobalValueType == types.UNDEFINED) {
		globalsDeclareResults(prgrm, names, spec, values, passOneValues)
		return
	}
	if initialized && count != len(names) {
		file, line, column := ast.CXArgumentDebugPosition(prgrm, names[0].ArgDetails)
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeAssignmentCount, file, line, column, ast.CompileErrorOperands{int64(len(names)), int64(count)})
		initialized = false
		if spec == nil {
			// The names are invalid, so their uses report nothing more.
			for _, name := range names {
				if !GlobalRedeclarationCheck(prgrm, name) {
					DeclareGlobalInPackage(prgrm, nil, name, DeclarationSpecifiersBasic(prgrm, types.INVALID), nil, false)
				}
			}
			return
		}
	}
	for i, name := range names {
		nameSpec := spec
		if spec != nil && i < len(names)-1 {
			nameSpec = argumentCopy(prgrm, spec)
		}
		if nameSpec == nil && passOneValues != nil {
			code := passOneValues[i].GlobalValueType
			if code == types.UNDEFINED {
				if !GlobalRedeclarationCheck(prgrm, name) {
					globalDeferredAdd(prgrm, name, passOneValues[i], 0, 0)
				}
				continue
			}
			nameSpec = DeclarationSpecifiersBasic(prgrm, code)
		} else if nameSpec == nil {
			glbl := globalDeclared(prgrm, name)
			if glbl == nil {
				file, line, column := ast.CXArgumentDebugPosition(prgrm, name.ArgDetails)
				prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeGlobalTypeUnknown, file, line, column, ast.CompileErrorOperands{int64(name.Name)})
				// Its uses report nothing more.
				nameSpec = DeclarationSpecifiersBasic(prgrm, types.INVALID)
			} else {
				if !GlobalRedeclarationCheck(prgrm, name) && initialized && values[i] != nil {
					processGlobalInitializer(prgrm, values[i], glbl.Index)
				}
				prgrm.ReleaseCXArgBuilder(name)
				continue
			}
		}
		if GlobalRedeclarationCheck(prgrm, name) {
			continue
		}
		var initializer []ast.CXExpression
		if initialized && values != nil {
			initializer = values[i]
		}
		DeclareGlobalInPackage(prgrm, nil, name, nameSpec, initializer, initialized && values != nil)
	}
}

// globalsDeclareResults declares the globals of var a, b = f() and of var
// a, b T = f(), each of which takes one result of f. Pass one declares them,
// with the inference pass for a value without a type, and pass two makes the
// call write every global, which the count of its results checks.
func globalsDeclareResults(prgrm *ast.CXProgram, names []*ast.CXArgument, spec *ast.CXArgument, values [][]ast.CXExpression, passOneValues []GlobalValue) {
	if passOneValues != nil {
		for i, name := range names {
			if GlobalRedeclarationCheck(prgrm, name) {
				continue
			}
			if spec == nil {
				globalDeferredAdd(prgrm, name, passOneValues[0], i, len(names))
				continue
			}
			nameSpec := spec
			if i < len(names)-1 {
				nameSpec = argumentCopy(prgrm, spec)
			}
			DeclareGlobalInPackage(prgrm, nil, name, nameSpec, nil, false)
		}
		return
	}
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	var globalIdxs []ast.CXTypeSignatureIndex
	var others []stringpool.StringGid
	for i, name := range names {
		glbl, err := pkg.GetGlobal(prgrm, name.Name)
		if err != nil || typeSignatureInvalid(prgrm, glbl) {
			// The global has its error, so the value reports nothing more.
			return
		}
		globalIdxs = append(globalIdxs, glbl.Index)
		if i > 0 {
			others = append(others, name.Name)
		}
	}
	value := values[0]
	call := &prgrm.CXAtomicOps[value[len(value)-1].Index]
	operator := prgrm.GetFunctionFromArray(call.Operator)
	results := 1
	if operator != nil && len(call.GetOutputs(prgrm)) == 0 {
		results = len(operator.GetOutputs(prgrm))
	}
	if results != len(names) {
		file, line, column := ast.CXArgumentDebugPosition(prgrm, names[0].ArgDetails)
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeAssignmentCount, file, line, column, ast.CompileErrorOperands{int64(len(names)), int64(results)})
		return
	}
	start := len(prgrm.SysInitExprs)
	call.Outputs = nil
	for _, glblIdx := range globalIdxs {
		call.AddOutput(prgrm, glblIdx)
	}
	prgrm.SysInitExprs = append(prgrm.SysInitExprs, value...)
	prgrm.CXProgramGlobalInitializerAdd(prgrm.CurrentPackage, names[0].Name, start, others...)
	for _, name := range names {
		prgrm.ReleaseCXArgBuilder(name)
	}
}

// globalDeferredAdd records the global name of var name = value, whose value
// is not a literal, for the inference pass of t9-cxparsing. The global of
// var a, b = f() takes the result at result of f, which has results.
func globalDeferredAdd(prgrm *ast.CXProgram, name *ast.CXArgument, value GlobalValue, result int, results int) {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	file, _, _ := ast.CXArgumentDebugPosition(prgrm, name.ArgDetails)
	prgrm.CXProgramGlobalDeferredAdd(ast.GlobalDeferred{
		GlobalDeferredDeclarator:  ast.CXArgumentIndex(name.Index),
		GlobalDeferredPackage:     ast.CXPackageIndex(pkg.Index),
		GlobalDeferredFile:        file,
		GlobalDeferredStartLine:   value.GlobalValueStartLine,
		GlobalDeferredStartColumn: value.GlobalValueStartColumn,
		GlobalDeferredEndLine:     value.GlobalValueEndLine,
		GlobalDeferredEndColumn:   value.GlobalValueEndColumn,
		GlobalDeferredResult:      result,
		GlobalDeferredResults:     results,
	})
}

// globalDeclared is the global of the current package that name names when
// an earlier pass gave it its type, or nil.
func globalDeclared(prgrm *ast.CXProgram, name *ast.CXArgument) *ast.CXTypeSignature {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		return nil
	}
	glbl, err := pkg.GetGlobal(prgrm, name.Name)
	if err != nil {
		return nil
	}
	if glbl.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		if !prgrm.GetCXArg(ast.CXArgumentIndex(glbl.Meta)).Offset.IsValid() {
			return nil
		}
	} else if !glbl.Offset.IsValid() {
		return nil
	}
	return glbl
}

// DeclarationSpecifiersCopy copies the type spec of var a, b T for the
// second name and on.
func DeclarationSpecifiersCopy(prgrm *ast.CXProgram, spec *ast.CXArgument) *ast.CXArgument {
	return argumentCopy(prgrm, spec)
}

// globalPlaceholderAdd adds to pkg the global that declarator names, with no
// type yet, as Preliminarystage adds the globals it finds. The global then
// keeps its argument, as a global Preliminarystage found does, which the
// name of var ( g = T{...} ) or of var g = T{x: 1, on its own line is not.
func globalPlaceholderAdd(prgrm *ast.CXProgram, pkg *ast.CXPackage, declarator *ast.CXArgument) {
	file, line, column := ast.CXArgumentDebugPosition(prgrm, declarator.ArgDetails)
	_, arg := ast.MakeArgument(prgrm, declarator.Name, file, line)
	if arg.ArgDetails != ast.CXArgumentDebugGidNil {
		prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(arg.ArgDetails).FileColumn = column
	}
	arg.Offset = types.InvalidPointer
	arg.Package = ast.CXPackageIndex(pkg.Index)
	pkg.AddGlobal(prgrm, ast.CXArgumentIndex(arg.Index))
}
