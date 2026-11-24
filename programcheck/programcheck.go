// Package programcheck checks a CX program before it runs, without running
// it: the system libraries it calls, its network use, its imports, the
// opcode sets it uses and a few checks of the IR. The policy of the program
// decides which of those are allowed. See "Validation before running (#16)"
// in docs/ProgramDistribution.md.
package programcheck

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	types "github.com/skycoin/skycoin-cx/t0-types"
	constants "github.com/skycoin/skycoin-cx/t1-constants"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/manifest"
)

// OpcodeSet names a group of opcodes.
type OpcodeSet string

// The opcode sets. OpcodeSetCore is every opcode of skycoin-cx
// constants.OpcodeLifecycleTable except the library call. OpcodeSetApp is
// the app package of the playground, opcodes from cxrun.AppOpcodeBase.
// OpcodeSetLibrary is constants.OP_LIBRARY_CALL, the system libraries.
const (
	OpcodeSetCore    OpcodeSet = "core"
	OpcodeSetApp     OpcodeSet = "app"
	OpcodeSetLibrary OpcodeSet = "library"
)

// Policy is what a program may use.
type Policy struct {
	// OpcodeSets lists the opcode sets the program may use.
	OpcodeSets []OpcodeSet
	// Libraries lists the system libraries the program may call, by their
	// skycoin-cx names: file, net and process.
	Libraries []string
	// BlockedImports lists the packages the program may not import.
	BlockedImports []string
}

// PolicyBundled is the policy of the apps built into the app: everything.
func PolicyBundled() Policy {
	return Policy{
		OpcodeSets: []OpcodeSet{OpcodeSetCore, OpcodeSetApp, OpcodeSetLibrary},
		Libraries:  cxrun.LibrariesAll(),
	}
}

// PolicyFetched is the policy of a program that came over the network. It
// may draw and use the network through the app package, but not reach
// files, processes or raw sockets: the os and net packages are blocked and
// so are the system libraries.
func PolicyFetched() Policy {
	return Policy{
		OpcodeSets:     []OpcodeSet{OpcodeSetCore, OpcodeSetApp},
		BlockedImports: []string{"os", "net"},
	}
}

// LibraryCall counts the calls of one system library function.
type LibraryCall struct {
	Library  string
	Function string
	Calls    int
}

// Report is the result of a check.
type Report struct {
	// Form is the program form, manifest.FormSource, FormIR or FormForth.
	Form string
	// LibraryCalls lists the system library functions the program calls,
	// by library and then function.
	LibraryCalls []LibraryCall
	// NetworkCalls counts the app.NetDial calls, and UDP, TCP and
	// NetworkUnknown tell their network: "udp", "tcp", or a name that is
	// not a constant. A call of the net library counts as TCP.
	NetworkCalls   int
	UDP            bool
	TCP            bool
	NetworkUnknown bool
	// Imports lists the packages the program imports.
	Imports []string
	// BlockedImports lists the imports the policy blocks.
	BlockedImports []string
	// OpcodeSets lists the opcode sets the program uses, and OpcodeCounts
	// the number of operations of each.
	OpcodeSets   []OpcodeSet
	OpcodeCounts map[OpcodeSet]int
	// Functions and Operations count the program's functions and the
	// operations in them.
	Functions  int
	Operations int
	// IRProblems lists what the IR checks found: unknown opcodes, calls of
	// missing functions and argument counts that do not match.
	IRProblems []string
	// Violations lists every reason the program may not run under the
	// policy. A program runs only when it is empty.
	Violations []string
}

// Allowed reports whether the program may run under the policy it was
// checked against.
func (r *Report) Allowed() bool { return len(r.Violations) == 0 }

// LibraryUsed reports whether the program calls the system library name.
func (r *Report) LibraryUsed(name string) bool {
	return slices.ContainsFunc(r.LibraryCalls, func(c LibraryCall) bool { return c.Library == name })
}

// LibrariesToEnable returns the libraries the program calls that the
// policy allows, the ones to give cxrun.Options.Libraries.
func (r *Report) LibrariesToEnable(p Policy) []string {
	var names []string
	for _, name := range cxrun.LibrariesAll() {
		if r.LibraryUsed(name) && slices.Contains(p.Libraries, name) {
			names = append(names, name)
		}
	}
	return names
}

// ErrNotCX is returned for a Forth program, which has no CX IR to check.
var ErrNotCX = errors.New("not a CX program")

// CheckApp checks the program of an app, in its form, against p. A Forth
// program is not checked: its report has the form and no violations, and
// cx-forth has no system calls to check.
func CheckApp(m *manifest.Manifest, files map[string][]byte, p Policy) (*Report, error) {
	switch m.Form {
	case manifest.FormSource:
		return CheckSource(files, m.Entry, p)
	case manifest.FormIR:
		return CheckIR(files[m.Entry], p)
	case manifest.FormForth:
		return &Report{Form: manifest.FormForth}, nil
	}
	return nil, fmt.Errorf("unknown form %q", m.Form)
}

// CheckSource compiles the .cx files in files, entry first, and checks the
// compiled program against p.
func CheckSource(files map[string][]byte, entry string, p Policy) (*Report, error) {
	r := &Report{Form: manifest.FormSource}
	err := cxrun.InspectSource(files, entry, func(prgrm *cxast.CXProgram) error {
		r.walk(prgrm)
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.judge(p)
	return r, nil
}

// CheckIR checks a serialized CX program against p.
func CheckIR(program []byte, p Policy) (*Report, error) {
	r := &Report{Form: manifest.FormIR}
	err := cxrun.InspectIR(program, func(prgrm *cxast.CXProgram) error {
		r.walk(prgrm)
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.judge(p)
	return r, nil
}

// walk reads every operation of every function of prgrm.
func (r *Report) walk(prgrm *cxast.CXProgram) {
	r.OpcodeCounts = map[OpcodeSet]int{}
	calls := map[[2]string]int{}
	imports := map[string]bool{}
	for _, index := range prgrm.Packages {
		pkg, err := prgrm.GetPackageFromArray(index)
		if err != nil {
			continue
		}
		for name := range pkg.Imports {
			imports[prgrm.CXProgramNameText(name)] = true
		}
	}
	for name := range imports {
		r.Imports = append(r.Imports, name)
	}
	sort.Strings(r.Imports)

	for i := range prgrm.CXFunctions {
		fn := &prgrm.CXFunctions[i]
		if fn.AtomicOPCode != 0 || len(fn.Expressions) == 0 {
			continue
		}
		r.Functions++
		fnName := prgrm.CXProgramNameText(fn.Name)
		for _, expr := range fn.Expressions {
			if expr.Type != cxast.CX_ATOMIC_OPERATOR {
				continue
			}
			op, err := prgrm.GetCXAtomicOp(expr.Index)
			if err != nil {
				r.IRProblems = append(r.IRProblems, fmt.Sprintf("%s: %v", fnName, err))
				continue
			}
			if op.Operator == -1 {
				continue
			}
			r.Operations++
			if int(op.Operator) < 0 || int(op.Operator) >= len(prgrm.CXFunctions) {
				r.IRProblems = append(r.IRProblems, fmt.Sprintf("%s calls missing function %d", fnName, op.Operator))
				continue
			}
			callee := prgrm.GetFunctionFromArray(op.Operator)
			code := callee.AtomicOPCode
			switch {
			case code == 0:
				r.checkCall(prgrm, fnName, op, callee)
			case code == constants.OP_LIBRARY_CALL:
				r.OpcodeCounts[OpcodeSetLibrary]++
				row, found := constants.LibraryFunctionLiveByNumber(callee.LibraryNumber, callee.LibraryFunctionNumber)
				if !found {
					r.IRProblems = append(r.IRProblems, fmt.Sprintf("%s calls unknown function %d of library %d", fnName, callee.LibraryFunctionNumber, callee.LibraryNumber))
					continue
				}
				calls[[2]string{constants.LibraryNameOf(callee.LibraryNumber), row.Name}]++
				if callee.LibraryNumber == constants.LibraryNumberNet {
					r.TCP = true
				}
			case code >= cxrun.AppOpcodeBase:
				name := cxast.NativeNames.NativeNameIndexCodeText(code)
				if name == "" {
					r.IRProblems = append(r.IRProblems, fmt.Sprintf("%s uses unknown app opcode %d", fnName, code))
					continue
				}
				r.OpcodeCounts[OpcodeSetApp]++
				if name == "app.NetDial" {
					r.networkDial(prgrm, op)
				}
			default:
				if _, found := constants.OpcodeLifecycleLiveByNumber(code); !found {
					r.IRProblems = append(r.IRProblems, fmt.Sprintf("%s uses unknown opcode %d", fnName, code))
					continue
				}
				r.OpcodeCounts[OpcodeSetCore]++
			}
		}
	}
	for key, n := range calls {
		r.LibraryCalls = append(r.LibraryCalls, LibraryCall{Library: key[0], Function: key[1], Calls: n})
	}
	sort.Slice(r.LibraryCalls, func(i, j int) bool {
		a, b := r.LibraryCalls[i], r.LibraryCalls[j]
		if a.Library != b.Library {
			return a.Library < b.Library
		}
		return a.Function < b.Function
	})
	for _, set := range []OpcodeSet{OpcodeSetCore, OpcodeSetApp, OpcodeSetLibrary} {
		if r.OpcodeCounts[set] > 0 {
			r.OpcodeSets = append(r.OpcodeSets, set)
		}
	}
}

// checkCall checks that a call of a CX function passes as many arguments
// as the function takes.
func (r *Report) checkCall(prgrm *cxast.CXProgram, fnName string, op *cxast.CXAtomicOperator, callee *cxast.CXFunction) {
	if callee.Inputs == nil || op.Inputs == nil {
		return
	}
	if got, want := len(op.Inputs.Fields), len(callee.Inputs.Fields); got != want {
		r.IRProblems = append(r.IRProblems, fmt.Sprintf("%s calls %s with %d arguments, it takes %d", fnName, prgrm.CXProgramNameText(callee.Name), got, want))
	}
}

// networkDial records the network of an app.NetDial call from its first
// argument, when that is a string constant.
func (r *Report) networkDial(prgrm *cxast.CXProgram, op *cxast.CXAtomicOperator) {
	r.NetworkCalls++
	network, constant := "", false
	if op.Inputs != nil && len(op.Inputs.Fields) > 0 {
		network, constant = stringConstant(prgrm, op.Inputs.Fields[0])
	}
	switch {
	case constant && network == "udp":
		r.UDP = true
	case constant && network == "tcp":
		r.TCP = true
	default:
		r.NetworkUnknown = true
	}
}

// stringConstant returns the value of a str argument that is a literal: it
// has no name and sits in the data segment.
func stringConstant(prgrm *cxast.CXProgram, index cxast.CXTypeSignatureIndex) (value string, constant bool) {
	ts := prgrm.GetCXTypeSignatureFromArray(index)
	if ts == nil || prgrm.CXProgramNameText(ts.Name) != "" {
		return "", false
	}
	start, end := prgrm.Data.StartsAt, prgrm.Data.StartsAt+prgrm.Data.Size
	if ts.Offset < start || ts.Offset >= end || int(ts.Offset) >= len(prgrm.Memory) {
		return "", false
	}
	defer func() {
		if recover() != nil {
			value, constant = "", false
		}
	}()
	return types.Read_str(prgrm.Memory, ts.Offset), true
}

// judge fills Violations from what walk found and the policy.
func (r *Report) judge(p Policy) {
	for _, name := range r.Imports {
		if slices.Contains(p.BlockedImports, name) {
			r.BlockedImports = append(r.BlockedImports, name)
			r.Violations = append(r.Violations, "import "+name+" is blocked")
		}
	}
	for _, set := range r.OpcodeSets {
		if !slices.Contains(p.OpcodeSets, set) {
			r.Violations = append(r.Violations, fmt.Sprintf("opcode set %s is not allowed (%d operations)", set, r.OpcodeCounts[set]))
		}
	}
	for _, name := range cxrun.LibrariesAll() {
		if r.LibraryUsed(name) && !slices.Contains(p.Libraries, name) {
			r.Violations = append(r.Violations, "library "+name+" is not allowed")
		}
	}
	r.Violations = append(r.Violations, r.IRProblems...)
}

// Text is the report as short monospaced lines, one per check, the screen
// shows before a run.
func (r *Report) Text() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	yesNo := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}
	if r.Form == manifest.FormForth {
		line("form forth, runs on cx-forth, which has no system calls")
		line("sandbox violations none")
		line("verdict run")
		return b.String()
	}
	form := "source"
	if r.Form == manifest.FormIR {
		form = "ir"
	}
	line("form %s, %d functions, %d operations", form, r.Functions, r.Operations)
	line("file system %s", yesNo(r.LibraryUsed("file")))
	networking := r.NetworkCalls > 0 || r.LibraryUsed("net")
	if networking {
		udp, tcp := yesNo(r.UDP), yesNo(r.TCP)
		if r.NetworkUnknown {
			if !r.UDP {
				udp = "unknown"
			}
			if !r.TCP {
				tcp = "unknown"
			}
		}
		line("networking yes, udp %s, tcp %s", udp, tcp)
	} else {
		line("networking no")
	}
	line("process %s", yesNo(r.LibraryUsed("process")))
	if len(r.LibraryCalls) == 0 {
		line("library calls none")
	} else {
		var calls []string
		for _, c := range r.LibraryCalls {
			calls = append(calls, fmt.Sprintf("%s %s", c.Library, c.Function))
		}
		line("library calls %s", strings.Join(calls, ", "))
	}
	line("imports %s", listOrNone(r.Imports))
	line("blocked imports %s", listOrNone(r.BlockedImports))
	var sets []string
	for _, set := range r.OpcodeSets {
		sets = append(sets, fmt.Sprintf("%s %d", set, r.OpcodeCounts[set]))
	}
	line("opcode sets %s", listOrNoneSep(sets, ", "))
	if len(r.IRProblems) == 0 {
		line("ir checks ok")
	} else {
		line("ir checks %d problems", len(r.IRProblems))
	}
	if len(r.Violations) == 0 {
		line("sandbox violations none")
		line("verdict run")
	} else {
		line("sandbox violations %d", len(r.Violations))
		for _, v := range r.Violations {
			line("  %s", v)
		}
		line("verdict refused")
	}
	return b.String()
}

func listOrNone(items []string) string { return listOrNoneSep(items, " ") }

func listOrNoneSep(items []string, sep string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, sep)
}
