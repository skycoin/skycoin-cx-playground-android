package parsingcompletor

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-actions"
)

// ParserActionAssignmentExpression is the action of an assignment, shared by
// assignment_expression and the for clauses.
func ParserActionAssignmentExpression(left []ast.CXExpression, operator string, right []ast.CXExpression) []ast.CXExpression {
	if operator == ":=" {
		left = actions.ImportScopedShortDeclaration(actions.AST, left)
	}
	result := left

	if right == nil {
		result = nil
	}
	if right != nil {
		for i := 0; i < len(right); i++ {
			if right[i].Type != ast.CX_LINE {
				if right[i].IsArrayLiteral() {
					if operator != "=" && operator != ":=" {
						panic("")
					}
					if operator == ":=" {
						for _, from := range right {
							if from.Type == ast.CX_LINE {
								continue
							}

							// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
							// Now an arena getter, site S1493.
							fromExpression, err := actions.AST.GetCXAtomicOp(from.Index)
							if err != nil {
								panic(err)
							}

							// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
							// Now an arena getter, site S1494.
							fromExpressionOutputTypeSig := actions.AST.GetCXTypeSignatureFromArray(fromExpression.GetOutputs(actions.AST)[0])
							fromExpressionOutputIdx := fromExpressionOutputTypeSig.Meta
							// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
							// Now an arena getter, site S1773.
							actions.AST.GetCXArg(ast.CXArgumentIndex(fromExpressionOutputIdx)).PreviouslyDeclared = true
						}
					}
					result = actions.ArrayLiteralAssignment(actions.AST, left, right)
				} else if right[len(right)-1].IsStructLiteral() {
					if operator != "=" && operator != ":=" {
						panic("")
					}
					if operator == ":=" {
						for _, from := range right {
							if from.Type == ast.CX_LINE {
								continue
							}

							// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
							// Now an arena getter, site S1495.
							fromExpression, err := actions.AST.GetCXAtomicOp(from.Index)
							if err != nil {
								panic(err)
							}

							// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
							// Now an arena getter, site S1496.
							fromExpressionOutputTypeSig := actions.AST.GetCXTypeSignatureFromArray(fromExpression.GetOutputs(actions.AST)[0])
							fromExpressionOutputIdx := fromExpressionOutputTypeSig.Meta
							// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
							// Now an arena getter, site S1774.
							actions.AST.GetCXArg(ast.CXArgumentIndex(fromExpressionOutputIdx)).PreviouslyDeclared = true
						}
					}
					result = actions.StructLiteralAssignment(actions.AST, left, right)
				} else {
					result = actions.AssignmentStatement(actions.AST, left, operator, right)
				}

				break
			}
		}
	}
	return result
}

// ParserActionExpressionStatement is the action of an expression statement,
// shared by expression_statement and the for clauses.
func ParserActionExpressionStatement(exprs []ast.CXExpression) []ast.CXExpression {
	exprs = actions.MethodCallStatement(actions.AST, exprs)
	var result []ast.CXExpression
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
	// Now a heap literal, site S1497.
	var lastFirstAtomicOp *ast.CXAtomicOperator = &ast.CXAtomicOperator{Operator: -1}
	var err error

	if len(exprs) > 0 {
		lastFirstAtomicOp, err = actions.AST.GetCXAtomicOpFromExpressions(exprs, len(exprs)-1)
		if err != nil {
			panic(err)
		}
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S1498.
	lastFirstAtomicOpOperator := actions.AST.GetFunctionFromArray(lastFirstAtomicOp.Operator)

	if len(exprs) > 0 && lastFirstAtomicOpOperator == nil && !exprs[len(exprs)-1].IsMethodCall() && !actions.ExpressionsDeclareInvalid(actions.AST, exprs) {
		outs := lastFirstAtomicOp.GetOutputs(actions.AST)

		if len(outs) > 0 {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1499.
			outTypeSig := actions.AST.GetCXTypeSignatureFromArray(outs[0])
			actions.AST.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidExpression, outTypeSig.ArgDetails, ast.CompileErrorOperands{})
		} else {
			actions.AST.CXProgramCompileErrorAdd(ast.CompileErrorCodeInvalidExpression, actions.CurrentFile, actions.LineNo, ast.CompileErrorOperands{})
		}
		result = nil
	} else if len(exprs) > 0 && exprs[0].Type != ast.CX_LINE {
		// test() and assert report the CX_LINE before them (skycoin-cx#254)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Also state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346. Site S1500 S1501.
		result = append([]ast.CXExpression{*actions.MakeCXLineExpressionCurrent(actions.AST)}, exprs...)
	} else {
		result = exprs
	}
	return result
}

// ParserActionLoopCondition keeps a literal or variable loop condition as
// it is, which IterationExpressions reads in place (skycoin-cx#21).
func ParserActionLoopCondition(exprs []ast.CXExpression) []ast.CXExpression {
	if len(exprs) > 0 && exprs[len(exprs)-1].Type != ast.CX_LINE && !exprs[len(exprs)-1].IsMethodCall() {
		cxAtomicOp, err := actions.AST.GetCXAtomicOpFromExpressions(exprs, len(exprs)-1)
		if err != nil {
			panic(err)
		}
		if actions.AST.GetFunctionFromArray(cxAtomicOp.Operator) == nil {
			return exprs
		}
	}
	return ParserActionExpressionStatement(exprs)
}

// ParserActionLabeledStatement labels every expression of a statement that
// has no label yet, so a nested goto keeps its target and a nested label stays.
func ParserActionLabeledStatement(label string, exprs []ast.CXExpression) []ast.CXExpression {
	labelGid := actions.AST.CXProgramNameIntern(label)
	first := -1
	labeled := false
	for i, expr := range exprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		if first < 0 {
			first = i
		}
		if actions.AST.CXAtomicOps[expr.Index].Label == stringpool.StringGidNil {
			actions.AST.CXAtomicOps[expr.Index].Label = labelGid
			labeled = true
		}
	}
	if !labeled && first >= 0 {
		actions.AST.CXAtomicOps[exprs[first].Index].Label = labelGid
	}
	return exprs
}

// ParserActionMultipleAssignment is the action of a, b = x, y and a, b := x, y.
// Every value is held in a variable before any target is assigned, as in Go.
func ParserActionMultipleAssignment(targets [][]ast.CXExpression, operator string, values [][]ast.CXExpression) []ast.CXExpression {
	if operator == ":=" {
		for index := range targets {
			targets[index] = actions.ImportScopedShortDeclaration(actions.AST, targets[index])
		}
	}
	for _, exprs := range append(append([][]ast.CXExpression(nil), targets...), values...) {
		if len(exprs) == 0 {
			return nil
		}
	}
	if len(values) == 1 {
		result := actions.MultipleAssignmentCall(actions.AST, targets, operator, values[0])
		if operator == ":=" {
			actions.ShortDeclarationShare(actions.AST, targets, result)
		}
		return result
	}
	if len(values) != len(targets) {
		actions.AST.CXProgramCompileErrorAdd(ast.CompileErrorCodeAssignmentCount, actions.CurrentFile, actions.LineNo, ast.CompileErrorOperands{int64(len(targets)), int64(len(values))})
		if operator == ":=" {
			return actions.ShortDeclarationTargetsInvalid(actions.AST, targets)
		}
		return nil
	}

	var result []ast.CXExpression
	names := make([]stringpool.StringGid, len(values))
	for i, value := range values {
		names[i] = actions.MultipleValueName(actions.AST)
		actions.SingleValueCheck(actions.AST, value)
		result = append(result, ParserActionAssignmentExpression(actions.MultipleValueRead(actions.AST, names[i]), ":=", value)...)
	}
	start := len(result)
	for i, target := range targets {
		result = append(result, ParserActionAssignmentExpression(target, operator, actions.MultipleValueRead(actions.AST, names[i]))...)
	}
	if operator == ":=" {
		actions.ShortDeclarationShare(actions.AST, targets, result[start:])
	}
	return result
}

// ParserActionDeclareLocals declares the locals of var a, b T and var a, b T =
// x, y, which evaluates every value before it assigns any, as in Go. A spec
// without a type, var a, b = x, y, is a, b := x, y.
func ParserActionDeclareLocals(names []*ast.CXArgument, spec *ast.CXArgument, values [][]ast.CXExpression, initialized bool) []ast.CXExpression {
	actions.ImportScopedLocalsDeclare(actions.AST, names)
	if spec != nil && (!initialized || len(names) == 1 && len(values) == 1) {
		var result []ast.CXExpression
		for i, name := range names {
			nameSpec := spec
			if i < len(names)-1 {
				nameSpec = actions.DeclarationSpecifiersCopy(actions.AST, spec)
			}
			var initializer []ast.CXExpression
			if initialized {
				initializer = values[0]
			}
			result = append(result, actions.DeclareLocal(actions.AST, name, nameSpec, initializer, initialized)...)
		}
		return result
	}

	_, line, column := ast.CXArgumentDebugPosition(actions.AST, names[0].ArgDetails)
	position := actions.TokenPositionPush(line, column)
	defer actions.TokenPositionPop(position)
	var result []ast.CXExpression
	targets := make([][]ast.CXExpression, len(names))
	for i, name := range names {
		targets[i] = actions.PrimaryIdentifier(actions.AST, actions.AST.CXProgramNameText(name.Name))
		if spec == nil {
			continue
		}
		nameSpec := spec
		if i < len(names)-1 {
			nameSpec = actions.DeclarationSpecifiersCopy(actions.AST, spec)
		}
		result = append(result, actions.DeclareLocal(actions.AST, name, nameSpec, nil, false)...)
	}
	if spec != nil {
		if len(targets) == 1 && len(values) == 1 {
			return append(result, ParserActionAssignmentExpression(targets[0], "=", values[0])...)
		}
		return append(result, ParserActionMultipleAssignment(targets, "=", values)...)
	}
	if len(targets) == 1 && len(values) == 1 {
		actions.AST.ReleaseCXArgBuilder(names[0])
		return ParserActionAssignmentExpression(targets[0], ":=", values[0])
	}
	if result = ParserActionMultipleAssignment(targets, ":=", values); result != nil {
		for _, name := range names {
			actions.AST.ReleaseCXArgBuilder(name)
		}
		return result
	}
	// The names of var a, b = 1 are invalid, so their uses report nothing more
	// than the count, as the global form does.
	for _, name := range names {
		result = append(result, actions.DeclareLocal(actions.AST, name, actions.DeclarationSpecifiersBasic(actions.AST, types.INVALID), nil, false)...)
	}
	return result
}

// ParserActionStructLiteralComma reports T{,}, a struct literal with no
// fields that ends in a comma, which Go does not allow.
func ParserActionStructLiteralComma(fields []ast.CXExpression, comma bool) {
	if comma && len(fields) == 0 {
		actions.AST.CXProgramCompileErrorAddLine(ast.CompileErrorCodeSyntax, actions.CurrentFile, actions.LineNo, ast.CompileErrorOperands{actions.AST.CXProgramCompileErrorText("unexpected comma, expected name")})
	}
}
