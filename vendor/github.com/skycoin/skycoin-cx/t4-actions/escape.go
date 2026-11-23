package actions

import (
	"slices"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// Escape analysis runs on the parsed body, before symbols resolve, and moves
// a local whose address outlives the call to the heap. LanguageGuide.md lists the rules.

// escapeKey names a location the analysis follows. A pointee key stands for
// whatever a pointer input of the function points to.
type escapeKey struct {
	escapeKeyName    stringpool.StringGid
	escapeKeyPointee bool
}

// escapeState is the analysis of one function.
type escapeState struct {
	escapeStatePrgrm *ast.CXProgram
	// escapeStateLocals are the declaration specifiers of each local and
	// input, inner type first.
	escapeStateLocals map[stringpool.StringGid][]int
	// escapeStateStructs is the struct type of each local of one.
	escapeStateStructs map[stringpool.StringGid]*ast.CXStruct
	escapeStateResults map[stringpool.StringGid]bool
	// escapeStatePoints holds, for each local, the locations whose address
	// it may hold.
	escapeStatePoints map[escapeKey]map[escapeKey]bool
	// escapeStateDeclared holds the expressions that declare each local, and
	// escapeStateLoops the first and last expression of each loop.
	escapeStateDeclared map[stringpool.StringGid][]int
	escapeStateLoops    [][2]int
	escapeStateEscaped  map[escapeKey]bool
	escapeStateChanged  bool
}

// escapeParam is an input of a function: its name and the specifiers and
// signature that rename it.
type escapeParam struct {
	escapeParamName      stringpool.StringGid
	escapeParamArgument  *ast.CXArgument
	escapeParamSignature *ast.CXTypeSignature
}

// escapeAnalyze returns the names of the locals and inputs whose address
// escapes, and where each input may keep the address it holds.
func escapeAnalyze(prgrm *ast.CXProgram, params []escapeParam, results []*ast.CXArgument, exprs []ast.CXExpression) (map[stringpool.StringGid]bool, []ast.CXFunctionEscape) {
	state := &escapeState{
		escapeStatePrgrm:    prgrm,
		escapeStateLocals:   map[stringpool.StringGid][]int{},
		escapeStateStructs:  map[stringpool.StringGid]*ast.CXStruct{},
		escapeStateResults:  map[stringpool.StringGid]bool{},
		escapeStatePoints:   map[escapeKey]map[escapeKey]bool{},
		escapeStateDeclared: map[stringpool.StringGid][]int{},
		escapeStateEscaped:  map[escapeKey]bool{},
	}
	for _, param := range params {
		key := escapeKey{escapeKeyName: param.escapeParamName}
		state.escapeStateLocals[param.escapeParamName] = escapeParamSpecifiers(prgrm, param)
		if param.escapeParamArgument != nil {
			state.escapeStateStructs[param.escapeParamName] = param.escapeParamArgument.StructType
		} else {
			state.escapeStateStructs[param.escapeParamName] = param.escapeParamSignature.GetStructType(prgrm)
		}
		state.escapePointsAdd(key, map[escapeKey]bool{{escapeKeyName: param.escapeParamName, escapeKeyPointee: true}: true})
	}
	for _, result := range results {
		name := StripNameNumber(prgrm, result.Name)
		state.escapeStateResults[name] = true
		state.escapeStateLocals[name] = result.DeclarationSpecifiers
		state.escapeStateStructs[name] = result.StructType
	}
	for i := range exprs {
		op := escapeAtomicOp(prgrm, &exprs[i])
		if op == nil {
			continue
		}
		if operator := prgrm.GetFunctionFromArray(op.Operator); operator != nil {
			state.escapeLoopAdd(i, op, operator)
			continue
		}
		for _, outputIdx := range op.GetOutputs(prgrm) {
			sig := prgrm.GetCXTypeSignatureFromArray(outputIdx)
			name := StripNameNumber(prgrm, sig.Name)
			if !state.escapeStateResults[name] {
				state.escapeStateLocals[name] = escapeSignatureSpecifiers(prgrm, sig)
				state.escapeStateDeclared[name] = append(state.escapeStateDeclared[name], i)
				state.escapeStateStructs[name] = sig.GetStructType(prgrm)
			}
		}
	}

	for {
		state.escapeStateChanged = false
		for i := range exprs {
			state.escapeExpression(&exprs[i])
		}
		for name := range state.escapeStateResults {
			for held := range state.escapeHeld(escapeKey{escapeKeyName: name}) {
				if !held.escapeKeyPointee {
					state.escapeAdd(map[escapeKey]bool{held: true})
				}
			}
		}
		for key := range state.escapeStateEscaped {
			state.escapeAdd(state.escapeClosure(key))
		}
		if !state.escapeStateChanged {
			break
		}
	}

	escaped := map[stringpool.StringGid]bool{}
	for key := range state.escapeStateEscaped {
		if !key.escapeKeyPointee && !state.escapeStateResults[key.escapeKeyName] {
			escaped[key.escapeKeyName] = true
		}
	}
	leaks := make([]ast.CXFunctionEscape, len(params))
	for i, param := range params {
		pointee := escapeKey{escapeKeyName: param.escapeParamName, escapeKeyPointee: true}
		if state.escapeStateEscaped[pointee] {
			leaks[i] = ast.CXFunctionEscapeHeap
			continue
		}
		for _, result := range results {
			if state.escapeClosure(escapeKey{escapeKeyName: StripNameNumber(prgrm, result.Name)})[pointee] {
				leaks[i] = ast.CXFunctionEscapeResult
			}
		}
	}
	return escaped, leaks
}

// escapeAtomicOp returns the operator of expr, or nil for a line.
func escapeAtomicOp(prgrm *ast.CXProgram, expr *ast.CXExpression) *ast.CXAtomicOperator {
	if expr.Type == ast.CX_LINE {
		return nil
	}
	op, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		return nil
	}
	return op
}

// escapeExpression follows the addresses one expression moves.
func (state *escapeState) escapeExpression(expr *ast.CXExpression) {
	prgrm := state.escapeStatePrgrm
	op := escapeAtomicOp(prgrm, expr)
	if op == nil {
		return
	}
	operator := prgrm.GetFunctionFromArray(op.Operator)
	inputs := op.GetInputs(prgrm)
	outputs := op.GetOutputs(prgrm)
	switch {
	case operator == nil:
	case expr.IsMethodCall() && len(inputs) > 0:
		state.escapeMethodCall(inputs, outputs)
		state.escapeStoreUnknown(outputs)
	case !operator.IsBuiltIn():
		for i, inputIdx := range inputs {
			state.escapeCallArgument(operator, i, state.escapeSources(inputIdx), outputs)
		}
		state.escapeStoreUnknown(outputs)
	case operator.AtomicOPCode == constants.OP_IDENTITY:
		if len(inputs) == 1 && len(outputs) == 1 {
			state.escapeStore(outputs[0], state.escapeSources(inputs[0]))
		}
	case escapeOpcodeStores(operator.AtomicOPCode):
		for _, inputIdx := range inputs {
			state.escapeAdd(state.escapeSources(inputIdx))
		}
		state.escapeStoreUnknown(outputs)
	default:
		state.escapeStoreUnknown(outputs)
	}
}

// escapeOpcodeStores reports whether a built-in stores its inputs in the
// heap, as append does.
func escapeOpcodeStores(opcode int) bool {
	switch ast.OpNames[opcode] {
	case "append", "insert", "resize", "remove", "copy":
		return true
	}
	return false
}

// escapeMethodCall follows the receiver and arguments of a method call. A
// value receiver of a method on a pointer gives its address.
func (state *escapeState) escapeMethodCall(inputs, outputs []ast.CXTypeSignatureIndex) {
	prgrm := state.escapeStatePrgrm
	method := state.escapeMethod(inputs[0])
	receiver := state.escapeSources(inputs[0])
	if method == nil {
		state.escapeAdd(receiver)
		state.escapeAdd(state.escapeAddressSources(inputs[0]))
	} else {
		methodInputs := method.GetInputs(prgrm)
		if len(methodInputs) > 0 && typeSignaturePointer(prgrm, prgrm.GetCXTypeSignatureFromArray(methodInputs[0])) {
			for key := range state.escapeAddressSources(inputs[0]) {
				receiver[key] = true
			}
		}
		state.escapeCallArgument(method, 0, receiver, outputs)
	}
	for i, inputIdx := range inputs[1:] {
		if method == nil {
			state.escapeAdd(state.escapeSources(inputIdx))
			continue
		}
		state.escapeCallArgument(method, i+1, state.escapeSources(inputIdx), outputs)
	}
}

// escapeMethod returns the method a receiver argument names, or nil when the
// receiver is not a local struct this analysis can type.
func (state *escapeState) escapeMethod(receiverIdx ast.CXTypeSignatureIndex) *ast.CXFunction {
	prgrm := state.escapeStatePrgrm
	sig := prgrm.GetCXTypeSignatureFromArray(receiverIdx)
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return nil
	}
	receiver := prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta))
	if len(receiver.Fields) != 1 {
		return nil
	}
	strct := state.escapeStateStructs[StripNameNumber(prgrm, receiver.Name)]
	if strct == nil {
		return nil
	}
	pkg, err := prgrm.GetPackageFromArray(strct.Package)
	if err != nil {
		return nil
	}
	fnIdx, err := pkg.GetMethod(prgrm, strct.Name, prgrm.GetCXArgFromArray(receiver.Fields[0]).Name)
	if err != nil {
		return nil
	}
	return prgrm.GetFunctionFromArray(fnIdx)
}

// escapeCallArgument handles sources passed as input i of fn. Only a function
// compiled already can say that it does not keep them.
func (state *escapeState) escapeCallArgument(fn *ast.CXFunction, i int, sources map[escapeKey]bool, outputs []ast.CXTypeSignatureIndex) {
	if len(sources) == 0 {
		return
	}
	if fn.EscapeInputs == nil || i >= len(fn.EscapeInputs) {
		state.escapeAdd(sources)
		return
	}
	switch fn.EscapeInputs[i] {
	case ast.CXFunctionEscapeHeap:
		state.escapeAdd(sources)
	case ast.CXFunctionEscapeResult:
		for _, outputIdx := range outputs {
			state.escapeStore(outputIdx, sources)
		}
	}
}

// escapeSources returns the locations whose address the value of an input
// may be.
func (state *escapeState) escapeSources(sigIdx ast.CXTypeSignatureIndex) map[escapeKey]bool {
	prgrm := state.escapeStatePrgrm
	sig := prgrm.GetCXTypeSignatureFromArray(sigIdx)
	if ast.CXTypeSignaturePassBy(prgrm, sig) == constants.PASSBY_REFERENCE {
		return state.escapeAddressSources(sigIdx)
	}
	name := StripNameNumber(prgrm, sig.Name)
	if _, found := state.escapeStateLocals[name]; !found {
		return escapeUnknownSources()
	}
	key := escapeKey{escapeKeyName: name}
	if escapeAccessInside(prgrm, state.escapeStateLocals[name], state.escapeStateStructs[name], sig) {
		if state.escapeStructLiteralTemporary(name, sig) {
			return map[escapeKey]bool{key: true}
		}
		return state.escapeHeld(key)
	}
	sources := map[escapeKey]bool{}
	for pointee := range state.escapeStatePoints[key] {
		if pointee.escapeKeyPointee {
			sources[escapeKeyUnknown()] = true
		}
		for held := range state.escapeHeld(pointee) {
			sources[held] = true
		}
	}
	return sources
}

// escapeAddressSources returns what an address of the input may point to,
// the local, or what the local points to when the access goes through it.
func (state *escapeState) escapeAddressSources(sigIdx ast.CXTypeSignatureIndex) map[escapeKey]bool {
	prgrm := state.escapeStatePrgrm
	sig := prgrm.GetCXTypeSignatureFromArray(sigIdx)
	name := StripNameNumber(prgrm, sig.Name)
	specs, found := state.escapeStateLocals[name]
	if !found {
		return escapeUnknownSources()
	}
	key := escapeKey{escapeKeyName: name}
	if escapeAccessInside(prgrm, specs, state.escapeStateStructs[name], sig) {
		return map[escapeKey]bool{key: true}
	}
	sources := state.escapeClosure(key)
	delete(sources, key)
	return sources
}

// escapeStore handles sources written to an output. A write inside a local
// makes the local hold them. Any other write lets them escape.
func (state *escapeState) escapeStore(outputIdx ast.CXTypeSignatureIndex, sources map[escapeKey]bool) {
	if len(sources) == 0 {
		return
	}
	prgrm := state.escapeStatePrgrm
	sig := prgrm.GetCXTypeSignatureFromArray(outputIdx)
	name := StripNameNumber(prgrm, sig.Name)
	specs, found := state.escapeStateLocals[name]
	if !found {
		state.escapeAdd(sources)
		return
	}
	key := escapeKey{escapeKeyName: name}
	if escapeAccessInside(prgrm, specs, state.escapeStateStructs[name], sig) {
		state.escapePointsAdd(key, sources)
		return
	}
	// A write through a pointer to locals only stays in them.
	pointees := state.escapeStatePoints[key]
	for pointee := range pointees {
		if pointee.escapeKeyPointee {
			state.escapeAdd(sources)
			return
		}
	}
	if len(pointees) == 0 {
		state.escapeAdd(sources)
		return
	}
	for pointee := range pointees {
		state.escapePointsAdd(pointee, sources)
	}
}

// escapePointsAdd records that holder may hold the addresses of sources.
func (state *escapeState) escapePointsAdd(holder escapeKey, sources map[escapeKey]bool) {
	for key := range sources {
		if state.escapeLoopOutlives(holder, key) {
			state.escapeAdd(map[escapeKey]bool{key: true})
		}
	}
	points := state.escapeStatePoints[holder]
	if points == nil {
		points = map[escapeKey]bool{}
		state.escapeStatePoints[holder] = points
	}
	for key := range sources {
		if !points[key] {
			points[key] = true
			state.escapeStateChanged = true
		}
	}
}

// escapeAdd marks sources as escaped.
func (state *escapeState) escapeAdd(sources map[escapeKey]bool) {
	for key := range sources {
		if !state.escapeStateEscaped[key] {
			state.escapeStateEscaped[key] = true
			state.escapeStateChanged = true
		}
	}
}

// escapeClosure returns key and every location reachable from it through
// the addresses locals hold.
func (state *escapeState) escapeClosure(key escapeKey) map[escapeKey]bool {
	closure := map[escapeKey]bool{key: true}
	pending := []escapeKey{key}
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for held := range state.escapeStatePoints[next] {
			if !closure[held] {
				closure[held] = true
				pending = append(pending, held)
			}
		}
	}
	return closure
}

// escapeAccessInside reports whether sig stays inside the storage of the
// local, as x, x.f and x[i] on an array do, and goes through no pointer or slice.
func escapeAccessInside(prgrm *ast.CXProgram, specs []int, strct *ast.CXStruct, sig *ast.CXTypeSignature) bool {
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return !sig.IsDeref
	}
	arg := prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta))
	stack, inside := escapeAccessIndexes(specs, arg)
	for _, fieldIdx := range arg.Fields {
		if !inside || strct == nil || len(stack) == 0 || stack[len(stack)-1] != constants.DECL_STRUCT {
			return false
		}
		field := prgrm.GetCXArgFromArray(fieldIdx)
		fieldSig, err := strct.GetField(prgrm, field.Name)
		if err != nil {
			return false
		}
		stack, inside = escapeAccessIndexes(escapeSignatureSpecifiers(prgrm, fieldSig), field)
		strct = fieldSig.GetStructType(prgrm)
	}
	return inside
}

// escapeAccessIndexes returns specs after the indexes of arg, and false when
// arg dereferences or indexes anything but an array.
func escapeAccessIndexes(specs []int, arg *ast.CXArgument) ([]int, bool) {
	indexes := len(arg.Indexes)
	indexing := 0
	for _, spec := range arg.DeclarationSpecifiers {
		switch spec {
		case constants.DECL_DEREF:
			return nil, false
		case constants.DECL_INDEXING:
			indexing++
		}
	}
	indexes = max(indexes, indexing)
	if indexes > len(specs) {
		return nil, false
	}
	for _, spec := range specs[len(specs)-indexes:] {
		if spec != constants.DECL_ARRAY {
			return nil, false
		}
	}
	return specs[:len(specs)-indexes], true
}

// escapeParamSpecifiers returns the declaration specifiers of an input.
func escapeParamSpecifiers(prgrm *ast.CXProgram, param escapeParam) []int {
	if param.escapeParamArgument != nil {
		return param.escapeParamArgument.DeclarationSpecifiers
	}
	return escapeSignatureSpecifiers(prgrm, param.escapeParamSignature)
}

// escapeSignatureSpecifiers returns the declaration specifiers of the
// variable a declaration signature declares.
func escapeSignatureSpecifiers(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) []int {
	array := func(base []int) []int {
		details := prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta)
		for range details.Lengths {
			base = append(base, constants.DECL_ARRAY)
		}
		return base
	}
	switch sig.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		return prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta)).DeclarationSpecifiers
	case ast.TYPE_POINTER_ATOMIC:
		return []int{constants.DECL_BASIC, constants.DECL_POINTER}
	case ast.TYPE_ARRAY_ATOMIC:
		return array([]int{constants.DECL_BASIC})
	case ast.TYPE_POINTER_ARRAY_ATOMIC:
		return append(array([]int{constants.DECL_BASIC}), constants.DECL_POINTER)
	case ast.TYPE_SLICE_ATOMIC:
		return []int{constants.DECL_BASIC, constants.DECL_SLICE}
	case ast.TYPE_POINTER_SLICE_ATOMIC:
		return []int{constants.DECL_BASIC, constants.DECL_SLICE, constants.DECL_POINTER}
	case ast.TYPE_STRUCT:
		return []int{constants.DECL_STRUCT}
	case ast.TYPE_POINTER_STRUCT:
		return []int{constants.DECL_STRUCT, constants.DECL_POINTER}
	}
	return []int{constants.DECL_BASIC}
}

// escapeSlotSize is the frame size of an escaped local, the slot that holds
// the address of its heap object.
const escapeSlotSize = types.POINTER_SIZE

// escapeInputPrefix starts the new name of an escaped input. The body then
// declares the old name as a local, a copy of the input, which can escape.
const escapeInputPrefix = "*esc_"

// escapePrologue renames each escaped input and returns exprs after the
// declarations that copy them.
func escapePrologue(prgrm *ast.CXProgram, params []escapeParam, escaped map[stringpool.StringGid]bool, exprs []ast.CXExpression) []ast.CXExpression {
	var prologue []ast.CXExpression
	for _, param := range params {
		if !escaped[param.escapeParamName] {
			continue
		}
		hidden := prgrm.CXProgramNameIntern(escapeInputPrefix + prgrm.CXProgramNameText(param.escapeParamName))
		source := param.escapeParamArgument
		if source == nil {
			source = param.escapeParamSignature.GetCXArgFormat(prgrm)
		}
		if param.escapeParamSignature != nil {
			param.escapeParamSignature.Name = hidden
		}
		_, line := ast.CXArgumentDebugFileLine(prgrm, source.ArgDetails)
		_, specs := ast.MakeArgument(prgrm, param.escapeParamName, CurrentFile, line)
		specsIndex, specsDetails := specs.Index, specs.ArgDetails
		*specs = *source
		specs.Index, specs.ArgDetails = specsIndex, specsDetails
		specs.Lengths = slices.Clone(source.Lengths)
		specs.DereferenceOperations = slices.Clone(source.DereferenceOperations)
		specs.DeclarationSpecifiers = slices.Clone(source.DeclarationSpecifiers)
		specs.Offset = 0
		specs.PassBy = constants.PASSBY_VALUE
		source.Name = hidden

		_, declarator := ast.MakeArgument(prgrm, param.escapeParamName, CurrentFile, line)
		prologue = append(prologue, DeclareLocal(prgrm, declarator, specs, primaryIdentifierExpressions(prgrm, hidden), true)...)
	}
	if prologue == nil {
		return exprs
	}
	return append(prologue, exprs...)
}

// escapeSlot is the frame slot of an escaped local, a root for the GC.
type escapeSlot struct {
	escapeSlotName   stringpool.StringGid
	escapeSlotOffset types.Pointer
}

// escapeDeclare gives a new escaped local the next frame slot, which holds
// the address of its heap object.
func escapeDeclare(prgrm *ast.CXProgram, symbolsData *SymbolsData, typeSig *ast.CXTypeSignature, sym *ast.CXArgument, offset *types.Pointer) {
	slot := *offset
	if sym != nil {
		sym.Name = typeSig.Name
		sym.Offset = slot | constants.ESCAPE_OFFSET_FLAG
	}
	typeSig.Offset = slot | constants.ESCAPE_OFFSET_FLAG
	updateSymbolsTableOffsetAdd(prgrm, typeSig, offset, escapeSlotSize)
	symbolsData.escapedSlots = append(symbolsData.escapedSlots, escapeSlot{escapeSlotName: typeSig.Name, escapeSlotOffset: slot})
}

// escapeFunction analyzes fn before its body compiles. It returns the escaped
// names and the body, after the copies of the escaped inputs.
func escapeFunction(prgrm *ast.CXProgram, fn *ast.CXFunction, inputs, outputs []*ast.CXArgument, exprs []ast.CXExpression) (map[stringpool.StringGid]bool, []ast.CXExpression) {
	var params []escapeParam
	if fnInputs := fn.GetInputs(prgrm); len(fnInputs) != len(inputs) && len(fnInputs) > 0 {
		receiver := prgrm.GetCXTypeSignatureFromArray(fnInputs[0])
		param := escapeParam{escapeParamName: StripNameNumber(prgrm, receiver.Name), escapeParamSignature: receiver}
		if receiver.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			param.escapeParamArgument = prgrm.GetCXArg(ast.CXArgumentIndex(receiver.Meta))
		}
		params = append(params, param)
	}
	for _, input := range inputs {
		params = append(params, escapeParam{escapeParamName: input.Name, escapeParamArgument: input})
	}
	escaped, leaks := escapeAnalyze(prgrm, params, outputs, exprs)
	fn.EscapeInputs = leaks
	if len(escaped) == 0 {
		return nil, exprs
	}
	return escaped, escapePrologue(prgrm, params, escaped, exprs)
}

// escapeHeld returns the locations whose address the value of key may hold,
// directly or through the addresses those hold.
func (state *escapeState) escapeHeld(key escapeKey) map[escapeKey]bool {
	held := map[escapeKey]bool{}
	for pointee := range state.escapeStatePoints[key] {
		for reached := range state.escapeClosure(pointee) {
			held[reached] = true
		}
	}
	return held
}

// escapeKeyUnknown stands for memory the analysis does not follow, such as
// the heap, globals and what a call returns.
func escapeKeyUnknown() escapeKey {
	return escapeKey{escapeKeyName: stringpool.StringGidNil, escapeKeyPointee: true}
}

// escapeUnknownSources is the value of memory the analysis does not follow.
func escapeUnknownSources() map[escapeKey]bool {
	return map[escapeKey]bool{escapeKeyUnknown(): true}
}

// escapeStoreUnknown records that outputs get values the analysis does not
// follow.
func (state *escapeState) escapeStoreUnknown(outputs []ast.CXTypeSignatureIndex) {
	for _, outputIdx := range outputs {
		state.escapeStore(outputIdx, escapeUnknownSources())
	}
}

// escapeStructLiteralTemporary reports whether sig reads the temporary of a
// &T{...}, a *T that StructLiteralAssignment declares to hold the T itself.
func (state *escapeState) escapeStructLiteralTemporary(name stringpool.StringGid, sig *ast.CXTypeSignature) bool {
	prgrm := state.escapeStatePrgrm
	specs := state.escapeStateLocals[name]
	if !IsTempVar(prgrm, name) || state.escapeStateStructs[name] == nil || len(specs) == 0 || specs[len(specs)-1] != constants.DECL_POINTER {
		return false
	}
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return true
	}
	arg := prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta))
	return len(arg.Fields) == 0 && len(arg.Indexes) == 0
}

// escapeLoopAdd records the loop that a backward jump at expression i closes.
func (state *escapeState) escapeLoopAdd(i int, op *ast.CXAtomicOperator, operator *ast.CXFunction) {
	if !operator.IsBuiltIn() {
		return
	}
	switch operator.AtomicOPCode {
	case constants.OP_JMP, constants.OP_GOTO, constants.OP_BREAK, constants.OP_CONTINUE:
	default:
		return
	}
	for _, lines := range []int{op.ThenLines, op.ElseLines} {
		if target := i + lines + 1; target <= i {
			state.escapeStateLoops = append(state.escapeStateLoops, [2]int{target, i})
		}
	}
}

// escapeLoopOutlives reports whether holder outlives a pass of a loop that
// declares key. Go lets such an address escape, so each pass gets its own.
func (state *escapeState) escapeLoopOutlives(holder escapeKey, key escapeKey) bool {
	if key.escapeKeyPointee || holder.escapeKeyPointee {
		return false
	}
	holderDeclared := state.escapeStateDeclared[holder.escapeKeyName]
	for _, loop := range state.escapeStateLoops {
		for _, declared := range state.escapeStateDeclared[key.escapeKeyName] {
			if declared < loop[0] || declared > loop[1] {
				continue
			}
			inside := len(holderDeclared) > 0
			for _, holderIndex := range holderDeclared {
				inside = inside && holderIndex >= loop[0] && holderIndex <= loop[1]
			}
			if !inside {
				return true
			}
		}
	}
	return false
}
