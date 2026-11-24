package ast

//go:generate go run ../t0-cx-core-objmodelgen -dir .

import (
	"fmt"
	"os"

	constants "github.com/skycoin/skycoin-cx/t1-constants"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

// CxIrState holds one manager of each IR object type and is the only owner of
// every IR object, see docs/ObjectModel.md. Until phase 5 of CXProgram a
// program reaches its state through CXProgram.CxIrState.
type CxIrState struct {
	StringPool stringpool.StringPool
	// SourceFileMng holds every source file path once, see #413.
	SourceFileMng SourceFileMng
	// CompileErrorList holds the compile errors found so far, see #413.
	CompileErrorList CompileErrorList
	// Names the compiler and the run start look up, interned by CxIrStateInit
	// so that they work on a frozen pool. Main names the main package and main.
	CxIrStateNameMain         stringpool.StringGid
	CxIrStateNameSysInit      stringpool.StringGid
	CxIrStateNameOsArgs       stringpool.StringGid
	CxIrStateNameOs           stringpool.StringGid
	CXProgramMng              CXProgramMng
	CXPackageMng              CXPackageMng
	CXFunctionMng             CXFunctionMng
	CXNativeFunctionMng       CXNativeFunctionMng
	CXStructMng               CXStructMng
	CXExpressionMng           CXExpressionMng
	CXAtomicOperatorMng       CXAtomicOperatorMng
	CXLineMng                 CXLineMng
	CXArgumentMng             CXArgumentMng
	CXArgumentDebugMng        CXArgumentDebugMng
	CXTypeSignatureMng        CXTypeSignatureMng
	CXTypeSignature_ArrayMng  CXTypeSignature_ArrayMng
	CXTypeSignature_StructMng CXTypeSignature_StructMng
	CXCallMng                 CXCallMng
	// CxIrStateGlobalsDeferred holds the globals whose type pass one cannot
	// read, until the inference pass types them.
	CxIrStateGlobalsDeferred []GlobalDeferred
	// CxIrStateGlobalInitializers holds the run of SysInitExprs of each global.
	CxIrStateGlobalInitializers []GlobalInitializer
	// CxIrStateNativeNames caches the built-ins CXProgramNativeFind found by
	// name Gids. It is a compile cache and is not serialized.
	CxIrStateNativeNames map[NativeNameKey]NativeNameFound
}

// CxIrStateManagerLen is the type tag of one manager's objects and its Len,
// for the teardown test. objmodel.GidTypeTagNameTable names the tag.
type CxIrStateManagerLen struct {
	TypeTag objmodel.TypeTag
	Len     int
}

// CxIrStateInit sets up every manager. One state holds one program, and the
// call stack has its fixed size. The other Caps are at least twice the most
// objects one compile stage allocates for any program in tests/ and
// examples/, so a stage never fills a manager. A manager no stage uses yet
// starts at 64.
func (state *CxIrState) CxIrStateInit() {
	state.StringPool.StringPoolInit(1024, 16384)
	state.SourceFileMng.SourceFileMngInit(16)
	state.CompileErrorList.CompileErrorListInit(16)
	state.CxIrStateNameMain = state.StringPool.StringPoolIntern([]byte(constants.MAIN_PKG))
	state.CxIrStateNameSysInit = state.StringPool.StringPoolIntern([]byte(constants.SYS_INIT_FUNC))
	state.CxIrStateNameOsArgs = state.StringPool.StringPoolIntern([]byte(constants.OS_ARGS))
	state.CxIrStateNameOs = state.StringPool.StringPoolIntern([]byte(constants.OS_PKG))
	state.CXProgramMng.CXProgramMngInit(1, false)
	state.CXPackageMng.CXPackageMngInit(64, true)
	state.CXFunctionMng.CXFunctionMngInit(64, true)
	state.CXNativeFunctionMng.CXNativeFunctionMngInit(64, true)
	state.CXStructMng.CXStructMngInit(64, true)
	state.CXExpressionMng.CXExpressionMngInit(64, true)
	state.CXAtomicOperatorMng.CXAtomicOperatorMngInit(64, true)
	state.CXLineMng.CXLineMngInit(2048, true)
	state.CXArgumentMng.CXArgumentMngInit(8192, true)
	state.CXArgumentDebugMng.CXArgumentDebugMngInit(4096, true)
	state.CXTypeSignatureMng.CXTypeSignatureMngInit(8192, true)
	state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayMngInit(1024, true)
	state.CXTypeSignature_StructMng.CXTypeSignature_StructMngInit(256, true)
	state.CXCallMng.CXCallMngInit(int(constants.CALLSTACK_SIZE), false)
}

// CxIrStateTeardown1 releases the program. Releasing every object the
// program owns through the owner relations is #300, once they are Gids.
func (state *CxIrState) CxIrStateTeardown1() {
	manager := &state.CXProgramMng
	for index, live := range manager.CXProgramMngLive {
		if live {
			generation := manager.CXProgramMngGenerations[index]
			manager.CXProgramRelease(CXProgramGid(objmodel.GidCompose(CXProgramGidTypeTag, generation, uint32(index))))
		}
	}
}

// CxIrStateTeardown2 tears every manager down, which asserts that its Len is 0.
func (state *CxIrState) CxIrStateTeardown2() {
	state.CXProgramMng.CXProgramMngTeardown()
	state.CXPackageMng.CXPackageMngTeardown()
	state.CXFunctionMng.CXFunctionMngTeardown()
	state.CXNativeFunctionMng.CXNativeFunctionMngTeardown()
	state.CXStructMng.CXStructMngTeardown()
	state.CXExpressionMng.CXExpressionMngTeardown()
	state.CXAtomicOperatorMng.CXAtomicOperatorMngTeardown()
	state.CXLineMng.CXLineMngTeardown()
	state.CXArgumentMng.CXArgumentMngTeardown()
	state.CXArgumentDebugMng.CXArgumentDebugMngTeardown()
	state.CXTypeSignatureMng.CXTypeSignatureMngTeardown()
	state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayMngTeardown()
	state.CXTypeSignature_StructMng.CXTypeSignature_StructMngTeardown()
	state.CXCallMng.CXCallMngTeardown()
}

// CxIrStateManagerGrowBoundary grows every growable manager whose free slots
// are under half of its Cap to the next power of two, and prints a warning
// for each. It moves the arrays, so it runs only where no Get pointer is
// held, between compile stages and main loop iterations, see
// docs/ObjectModel.md.
func (state *CxIrState) CxIrStateManagerGrowBoundary() {
	cxIrStateGrowWarn("CXProgramMng", state.CXProgramMng.CXProgramMngGrowBoundary)
	cxIrStateGrowWarn("CXPackageMng", state.CXPackageMng.CXPackageMngGrowBoundary)
	cxIrStateGrowWarn("CXFunctionMng", state.CXFunctionMng.CXFunctionMngGrowBoundary)
	cxIrStateGrowWarn("CXNativeFunctionMng", state.CXNativeFunctionMng.CXNativeFunctionMngGrowBoundary)
	cxIrStateGrowWarn("CXStructMng", state.CXStructMng.CXStructMngGrowBoundary)
	cxIrStateGrowWarn("CXExpressionMng", state.CXExpressionMng.CXExpressionMngGrowBoundary)
	cxIrStateGrowWarn("CXAtomicOperatorMng", state.CXAtomicOperatorMng.CXAtomicOperatorMngGrowBoundary)
	cxIrStateGrowWarn("CXLineMng", state.CXLineMng.CXLineMngGrowBoundary)
	cxIrStateGrowWarn("CXArgumentMng", state.CXArgumentMng.CXArgumentMngGrowBoundary)
	cxIrStateGrowWarn("CXArgumentDebugMng", state.CXArgumentDebugMng.CXArgumentDebugMngGrowBoundary)
	cxIrStateGrowWarn("CXTypeSignatureMng", state.CXTypeSignatureMng.CXTypeSignatureMngGrowBoundary)
	cxIrStateGrowWarn("CXTypeSignature_ArrayMng", state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayMngGrowBoundary)
	cxIrStateGrowWarn("CXTypeSignature_StructMng", state.CXTypeSignature_StructMng.CXTypeSignature_StructMngGrowBoundary)
	cxIrStateGrowWarn("CXCallMng", state.CXCallMng.CXCallMngGrowBoundary)
}

func cxIrStateGrowWarn(name string, grow func() (int, int)) {
	if oldCap, newCap := grow(); newCap != oldCap {
		fmt.Fprintf(os.Stderr, "warning: object model %s grew from Cap %d to %d\n", name, oldCap, newCap)
	}
}

// CxIrStateManagerLens returns the type tag and Len of each manager.
func (state *CxIrState) CxIrStateManagerLens() []CxIrStateManagerLen {
	return []CxIrStateManagerLen{
		{CXProgramGidTypeTag, state.CXProgramMng.CXProgramLen()},
		{CXPackageGidTypeTag, state.CXPackageMng.CXPackageLen()},
		{CXFunctionGidTypeTag, state.CXFunctionMng.CXFunctionLen()},
		{CXNativeFunctionGidTypeTag, state.CXNativeFunctionMng.CXNativeFunctionLen()},
		{CXStructGidTypeTag, state.CXStructMng.CXStructLen()},
		{CXExpressionGidTypeTag, state.CXExpressionMng.CXExpressionLen()},
		{CXAtomicOperatorGidTypeTag, state.CXAtomicOperatorMng.CXAtomicOperatorLen()},
		{CXLineGidTypeTag, state.CXLineMng.CXLineLen()},
		{CXArgumentGidTypeTag, state.CXArgumentMng.CXArgumentLen()},
		{CXArgumentDebugGidTypeTag, state.CXArgumentDebugMng.CXArgumentDebugLen()},
		{CXTypeSignatureGidTypeTag, state.CXTypeSignatureMng.CXTypeSignatureLen()},
		{CXTypeSignature_ArrayGidTypeTag, state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayLen()},
		{CXTypeSignature_StructGidTypeTag, state.CXTypeSignature_StructMng.CXTypeSignature_StructLen()},
		{CXCallGidTypeTag, state.CXCallMng.CXCallLen()},
	}
}
