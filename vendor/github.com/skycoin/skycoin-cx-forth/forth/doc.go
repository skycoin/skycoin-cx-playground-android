// Package forth is a small Forth with tagged cells and three valued logic.
//
// It is written for TinyGo on microcontrollers as well as for the host. All
// stacks, data memory and code space are fixed size arrays, there is no
// reflection, and output is formatted with strconv rather than fmt.
//
// # Cells
//
// A cell carries an AtomicType: Int8, Int16, Int32, Int64, Uint8, Uint16,
// Uint32, Uint64, Float32, Float64, Bool, String, Ternary or StructRef. A
// literal is an Int64 or a Float64, and AtomicTypeConvert makes the others.
// The arithmetic words are polymorphic. Integer operands must share a type,
// except that an Int64 takes the type of the other operand, and the result
// wraps to that type. Any float makes the result a float. >I32 and >F32
// narrow a value the way CX i32 and f32 arithmetic does.
//
// The AtomicType words, with their stack effects:
//
//	AtomicTypeInt64 ... AtomicTypeStructRef  ( -- t )  one per type, t is its id
//	AtomicTypeConvert  ( x t -- y )  NULL passes through
//	AtomicTypeOf       ( x -- t )
//	AtomicTypeNameOf   ( t -- s )   such as Int32
//
// # Registers
//
// There are three registers, Left, Middle and Right, and one of them is the
// focus. The registers and the data stack are independent. Each register
// holds one cell, whose type is the register's declared type. A new register
// is the Int64 0 and Left is the focus.
//
//	RegisterFocusLeft, RegisterFocusMiddle, RegisterFocusRight  ( -- )
//	RegisterFocusPushToStack   ( -- x )  copies the focus
//	RegisterFocusPopFromStack  ( x -- )  x must have the focus type
//	RegisterClear              ( -- )    the zero of the focus type
//	RegisterTypeSet            ( t -- )  sets the type and clears
//	RegisterTypeGet            ( -- t )
//	RegisterFocusAdd, RegisterFocusSubtract, RegisterFocusMultiply,
//	RegisterFocusDivide, RegisterFocusModulo  ( x -- )  focus = focus op x
//	RegisterFocusNegate        ( -- )
//
// The typed words fail with a type error when an operand does not have the
// focus type. The CX IR translator checks types before it emits any Forth,
// so in translated code the run time checks are asserts.
//
// # Structs
//
// A struct type is a list of ordinals, each with an AtomicType. A struct
// instance lives in the struct space, and a StructRef cell holds its struct
// type id and index, so a ref fits in one register or slot. The zero ref is
// nil. The struct table is itself made of built-in structs:
//
//	StructRef             id 0  ( Int64 struct type id, Int64 index )
//	StructTypeDescriptor  id 1  ( Int64 ordinal count, StructRef first StructOrdinal )
//	StructOrdinal         id 2  ( Int64 atomic type, StructRef next StructOrdinal or nil )
//
// Each struct type has a StructTypeDescriptor and a chain of StructOrdinal
// instances. User types get ids from 3 up.
//
//	StructDefineStart        ( -- )
//	StructDefineFieldAdd     ( t -- )    adds the next ordinal
//	StructDefineEnd          ( -- id )   submits the struct type
//	StructInstanceNew        ( id -- ref )  fields start at their zero
//	StructRefTypeID          ( ref -- id )
//	StructTypeDescriptorRef  ( id -- ref )
//	StructTypeCount          ( -- n )
//	StructTypeIDStructRef, StructTypeIDStructTypeDescriptor,
//	StructTypeIDStructOrdinal  ( -- id )
//
// For example struct abd { int x; int y; int y } has three ordinals:
//
//	StructDefineStart AtomicTypeInt32 StructDefineFieldAdd
//	AtomicTypeInt32 StructDefineFieldAdd AtomicTypeInt32 StructDefineFieldAdd
//	StructDefineEnd CONSTANT abd
//
// Mark and Restore forget struct types and instances made after the mark.
//
// # The struct cursor
//
// The cursor walks the ordinals of one instance. It is VM state of its own,
// beside the registers and the stack. StructCursorNext at the last ordinal
// and StructCursorPrevious at ordinal 0 stay put and push FALSE, so a loop
// over every field is BEGIN ... StructCursorNext NOT UNTIL.
//
//	StructCursorOpen         ( ref -- )   ordinal 0 of ref
//	StructCursorFirst        ( -- )
//	StructCursorLast         ( -- )
//	StructCursorNext         ( -- moved )
//	StructCursorPrevious     ( -- moved )
//	StructCursorOrdinal      ( -- n )
//	StructCursorOrdinalSet   ( n -- )
//	StructCursorOrdinalType  ( -- t )
//	StructCursorFieldGet     ( -- x )
//	StructCursorFieldSet     ( x -- )     x must have the ordinal's type
//	StructUnpack             ( ref -- x0 ... xn-1 )  ordinal 0 deepest
//	StructSpaceTop           ( -- n )
//	StructSpaceRelease       ( n -- )     frees instances made since n
//
// The struct table can be read with the cursor too, starting from
// StructTypeDescriptorRef.
//
// Word names are matched without regard to case and keep the case they were
// defined with.
//
// # Ternary logic
//
// TRUE, FALSE and NULL push ternary cells. They are stored in balanced form,
// FALSE=-1 NULL=0 TRUE=1, which makes Kleene's strong logic arithmetic:
//
//	AND      min(a, b)
//	OR       max(a, b)
//	NOT      -a
//	IMPLIES  max(-a, b)          Kleene
//	LIMPLIES min(1, 1 - a + b)   Lukasiewicz, NULL LIMPLIES NULL is TRUE
//	XOR      -(a * b)
//	EQV      a * b
//
// AND, OR and XOR on two integers stay bitwise, as in standard Forth. When
// either operand is ternary, integers are read as Forth flags, zero false and
// anything else true. Comparisons always return a ternary cell, and any NULL
// operand makes the result NULL. Arithmetic on any ternary cell, NULL
// included, is an error rather than a value. A comment in the thread cited
// below points out that + on ternary values is meaningless, and an unknown
// reading can still be compared, which is where NULL propagation matters.
// >I32 >INT >F32 >F64 are conversions, not arithmetic, and pass NULL
// through. KNOWN? NULL? TRUE? and FALSE? test a value and never return NULL.
//
// # The eForth vocabulary
//
// MAYBE TNOT TAND TOR TEQUIV TIMPLY TXOR and T. follow ternary.fs by Marc
// Petremann (ESP32forth), from the r/Forth thread "TERNARY LOGIC revisited"
// https://www.reddit.com/r/Forth/comments/1xy20la/ternary_logic_revisited/
// and its code
// https://raw.githubusercontent.com/MPETREMANN11/eForth-Windows/main/INFERENCE/ternary.fs
// which is adapted from https://rosettacode.org/wiki/Ternary_logic#Forth .
//
// That file encodes TRUE=-1 FALSE=0 MAYBE=1 in plain integers, so its AND
// and OR are bitwise and still give Kleene results. This VM's tagged cells
// use FALSE=-1 NULL=0 TRUE=1 instead. MAYBE here pushes the tagged NULL.
// The T words work on both. With two integer operands they compute the
// eForth definitions bit for bit and return an integer, so ESP32forth code
// ports unchanged. With a tagged operand they read integers in the eForth
// encoding, -1 true, 0 false, anything else maybe, and return a tagged cell.
// T. prints T, F or ? for either form. AND OR NOT read integers as Forth
// flags, the T words read them in the eForth encoding.
//
// testdata/ternary.fs is a verbatim copy that the tests load and run.
//
// # Conditionals on NULL
//
// IF runs its true branch only for TRUE. FALSE and NULL both take the ELSE
// branch, the same rule SQL uses for WHERE. UNTIL and WHILE follow the same
// rule, so a loop ends only on TRUE.
//
// TRI-IF branches three ways.
//
//	cond TRI-IF  true-part  ELSE  false-part  NULL-ELSE  null-part  THEN
//
// ELSE and NULL-ELSE are both optional. Without NULL-ELSE, NULL takes the
// ELSE branch exactly as IF does.
//
// # Words for translated code
//
// n FRAME opens n zeroed local slots, i L@ and i L! read and write them and
// UNFRAME closes the frame. LABEL n, GOTO n and JMP3 t f n give numbered
// labels inside one definition, which is what jump based IR needs.
//
// # Host words
//
// VM.Define adds a word that Go code implements, with PushString, PopString
// and PopInt for its arguments. This is how a program reaches the network,
// clocks or pins, none of which the VM touches itself.
//
// # Strings
//
// S\" takes the escapes \" \\ \n \r and \t. + joins two strings, = and the
// other comparisons compare their text, and >STR formats any cell. Strings
// live in a pool that is compacted between instructions once it grows, so a
// loop that makes strings forever runs in bounded memory.
package forth
