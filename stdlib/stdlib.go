// Package stdlib is the Vertex standard library core, as source.
//
// It is data, not code: the C++ runtime vcx compiles and the Vertex
// declarations vsc checks, embedded so that a compiler built with `go
// build` carries its own library and needs nothing installed beside
// it. Compiling any of it is somebody else's job — vsc/build turns the
// runtime into an object with vcx, for whatever target a program is
// being built for — which is why this package imports nothing and a
// front end that only typechecks can read it for free.
//
// See proposed.md for the design, and ABI.md for the contract the
// runtime and the compiler both have to keep.
package stdlib

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed runtime
var files embed.FS

// The runtime's entry points: what compiled code calls, spelled once so
// that the compiler and the runtime cannot disagree about them. The
// target's symbol prefix goes in front of each.
const (
	Alloc           = "vertex_alloc"
	BoxAllocate     = "vertex_box_allocate"
	ErrorBox        = "vertex_error_box"
	ErrorContents   = "vertex_error_contents"
	ErrorMatches    = "vertex_error_matches"
	ErrorProject    = "vertex_error_project"
	ErrorInMain     = "vertex_error_in_main"
	TryFailed       = "vertex_try_failed"
	AsyncMain       = "vertex_async_main"
	AsyncMainStatus = "vertex_async_main_status"
	TaskSpawn       = "vertex_task_spawn"
	TaskSleep       = "vertex_task_sleep"
	TaskYield       = "vertex_task_yield"
	TaskStart       = "vertex_task_start"
	TaskDetached    = "vertex_task_start_detached"
	TaskHop         = "vertex_task_hop"
	TaskOnMain      = "vertex_task_on_main"
	TaskAssumeMain  = "vertex_task_assume_main"
	TaskNeedsHop    = "vertex_task_needs_hop"
	TaskJoin        = "vertex_task_join"
	TaskCell        = "vertex_task_cell"
	TaskCellData    = "vertex_task_cell_contents"
	TaskCellTyped   = "vertex_task_cell_typed"
	TaskTypedData   = "vertex_task_cell_typed_contents"
	// The task allocator: an async function's frames, from a bump
	// pointer with stack discipline. Swift's swift_task_alloc.
	TaskAlloc   = "vertex_task_alloc"
	TaskDealloc = "vertex_task_dealloc"
	Dealloc     = "vertex_dealloc"
	Retain      = "vertex_retain"
	Release     = "vertex_release"

	StringLiteral = "vertex_string_literal"
	StringRetain  = "vertex_string_retain"
	StringRelease = "vertex_string_release"
	StringCount   = "vertex_string_count"
	StringConcat  = "vertex_string_concat"
	StringEqual   = "vertex_string_equal"
	StringLess    = "vertex_string_less"
	StringIsEmpty = "vertex_string_is_empty"

	StringHasPrefix = "vertex_string_has_prefix"
	StringHasSuffix = "vertex_string_has_suffix"
	StringContains  = "vertex_string_contains"
	StringAppend    = "vertex_string_append"
	// StringDecodingUTF8 is String(decoding:as: UTF8.self) over an array.
	StringDecodingUTF8 = "vertex_string_decoding_utf8"
	// IntegerParse and FloatParse are Int(_:) and Double(_:) over text.
	IntegerParse = "vertex_integer_parse"
	FloatParse   = "vertex_float_parse"

	StringDescription      = "vertex_string_description"
	StringDebugDescription = "vertex_string_debug_description"
	Describe               = "vertex_describe"

	ArrayAllocate = "vertex_array_allocate"
	ArrayCount    = "vertex_array_count"
	ArrayIsEmpty  = "vertex_array_is_empty"
	ArrayElement  = "vertex_array_element"
	// ArrayUniqueElements is where an array's elements are once its
	// storage is its variable's alone: withUnsafeMutableBufferPointer.
	ArrayUniqueElements = "vertex_array_unique_elements"
	// StringCString is a NUL-terminated copy of a String's bytes, which
	// StringCStringFree lets go: withCString.
	StringCString     = "vertex_string_cstring"
	StringCStringFree = "vertex_string_cstring_free"
	// StringFromCString is String(cString:).
	StringFromCString = "vertex_string_from_cstring"
	// ExistentialIs asks whether an existential holds exactly a type, and
	// ExistentialProject is where it keeps the value: `x as? T`.
	ExistentialIs      = "vertex_existential_is"
	ExistentialProject = "vertex_existential_project"
	// DynamicCast is `x as? T`: a copy of what x is, as a T, or failure.
	DynamicCast = "vertex_dynamic_cast"
	// StringUTF8Array is a String's utf8, as an array of its bytes.
	StringUTF8Array = "vertex_string_utf8_array"
	// ArrayAppendUTF8 appends a String's bytes to an array of them:
	// `bytes.append(contentsOf: s.utf8)` without the array in between.
	ArrayAppendUTF8 = "vertex_array_append_utf8"
	// ArrayRemoveAllKeeping empties an array, keeping its storage when
	// asked to and it is the array's own: removeAll(keepingCapacity:).
	ArrayRemoveAllKeeping = "vertex_array_remove_all_keeping"
	// StringEqualFold is ASCII case-insensitive equality of two Strings.
	StringEqualFold = "vertex_string_equal_fold"
	// ArrayRepeating is [T](repeating:count:), which takes the value.
	ArrayRepeating = "vertex_array_repeating"

	Print = "vertex_print"

	CommandLineArguments = "vertex_command_line_arguments"

	// Array mutation and queries. See runtime/collections.cpp.
	ArrayAppend          = "vertex_array_append"
	ArrayAppendContents  = "vertex_array_append_contents"
	ArrayAssign          = "vertex_array_assign"
	ArrayElementForWrite = "vertex_array_element_for_write"
	ArrayInsert          = "vertex_array_insert"
	ArrayRemoveAt        = "vertex_array_remove_at"
	ArrayRemoveLast      = "vertex_array_remove_last"
	ArrayPopLast         = "vertex_array_pop_last"
	ArrayRemoveAll       = "vertex_array_remove_all"
	ArrayFirst           = "vertex_array_first"
	ArrayLast            = "vertex_array_last"
	ArrayContains        = "vertex_array_contains"
	ArrayEqual           = "vertex_array_equal"

	// Hasher, whose state is core's struct and whose work is the runtime's.
	HasherInit     = "vertex_hasher_init"
	HasherCombine  = "vertex_hasher_combine"
	HasherFinalize = "vertex_hasher_finalize"

	// Fatal ends the program with a message, as a failed precondition does.
	Fatal = "vertex_fatal"

	// ValuesEqual is == on two values known by their metadata.
	ValuesEqual = "vertex_values_equal"

	// Dictionary and Set, one hash table.
	HashTableEmpty          = "vertex_hash_table_empty"
	HashTableCount          = "vertex_hash_table_count"
	HashTableIsEmpty        = "vertex_hash_table_is_empty"
	HashTableNext           = "vertex_hash_table_next"
	HashTableKeyAt          = "vertex_hash_table_key_at"
	DictionaryValueAt       = "vertex_dictionary_value_at"
	DictionaryGetDefault    = "vertex_dictionary_get_default"
	DictionaryInsertLiteral = "vertex_dictionary_insert_literal"
	DictionaryGet           = "vertex_dictionary_get"
	DictionarySet           = "vertex_dictionary_set"
	DictionaryRemove        = "vertex_dictionary_remove"
	SetInsert               = "vertex_set_insert"
	SetContains             = "vertex_set_contains"
	SetRemove               = "vertex_set_remove"

	// Value witnesses every instance of a generic type shares. See
	// runtime/generic.cpp.
	WitnessOptionalCopy        = "vertex_vw_optional_copy"
	WitnessOptionalDestroy     = "vertex_vw_optional_destroy"
	WitnessOptionalAssignCopy  = "vertex_vw_optional_assign_copy"
	WitnessOptionalAssignTake  = "vertex_vw_optional_assign_take"
	WitnessReferenceCopy       = "vertex_vw_reference_copy"
	WitnessReferenceDestroy    = "vertex_vw_reference_destroy"
	WitnessReferenceAssignCopy = "vertex_vw_reference_assign_copy"
	WitnessReferenceAssignTake = "vertex_vw_reference_assign_take"
	WitnessTake                = "vertex_vw_take"
	WitnessGetEnumTag          = "vertex_vw_get_enum_tag"
	WitnessStoreEnumTag        = "vertex_vw_store_enum_tag"

	// The bridge to modules swiftc built. See runtime/swift.cpp.
	SwiftStringToSwift   = "vertex_swift_string_to_swift"
	SwiftStringFromSwift = "vertex_swift_string_from_swift"
	SwiftStringRelease   = "vertex_swift_string_release"
	SwiftArrayToSwift    = "vertex_swift_array_to_swift"
	SwiftArrayFromSwift  = "vertex_swift_array_from_swift"
	SwiftRelease         = "vertex_swift_release"
)

// Metadata is the symbol of the record the runtime exports for a type
// the core declares, by the type's name: "Int", "String", "Any". The
// metadata itself is MetadataOffset bytes into the record, past the
// value witness table pointer.
func Metadata(name string) string { return "vertex_metadata_" + name }

// MetadataOffset is where in a record the metadata begins.
const MetadataOffset = 8

// The layouts compiled code reads directly, in bytes. Each is a field of
// a struct in runtime/include/vertex/abi.h, and build's ABI test holds
// the two together.
const (
	// A value witness table hangs one word below the metadata; destroy
	// is its second function and the flags word follows the size and
	// stride.
	WitnessTableOffset  = -8
	WitnessDestroy      = 0x8
	WitnessInitWithCopy = 0x10
	WitnessFlags        = 0x50
	WitnessAlignMask    = 0xff
	WitnessIsNonPOD     = 1 << 16
	WitnessIsNonInline  = 1 << 17
	ExistentialBuffer   = 0
	ExistentialMetadata = 3 * 8
	ExistentialInline   = 3 * 8

	// A String is two words, the reference-or-bytes second.
	StringBytes      = 16
	StringObjectWord = 1

	// An array's count is the word after its header; its elements begin
	// past the count, the capacity and the element metadata.
	ArrayCountWord = 16
	ArrayElements  = 40

	// The kinds of the records the compiler emits for types declared
	// nowhere.
	KindOptional = 0x202
	KindTuple    = 0x301

	// FieldIndirect is set in the low bit of an enum case's payload
	// record where the case is indirect: the value is a box, and the
	// payload is inside it past the header.
	FieldIndirect  = 1
	KindArray      = 0x800
	KindClass      = 0x801
	KindDictionary = 0x802
	KindSet        = 0x803
)

// The object header, in words and in bytes: a metadata pointer and a
// reference count. runtime/include/vertex/abi.h is the same fact in the
// language the runtime is written in.
const (
	HeaderWords = 2
	HeaderBytes = HeaderWords * 8
)

// An async context's header, before the frame the compiler lays out: the
// caller's context, and where to go when this function returns.
//
//	+0  parent
//	+8  resumeParent
//	+16 the frame
//
// The layout is Swift's -- swiftc reads its frame at context + 16 -- and
// runtime/include/vertex/abi.h says the same in C++. See
// docs/vertex_swift_async.md.
const (
	AsyncContextWords = 2
	AsyncContextBytes = AsyncContextWords * 8
)

// Runtime is the runtime's source tree: the translation unit to compile
// at its root, and the headers it includes under include/.
func Runtime() fs.FS {
	sub, err := fs.Sub(files, "runtime")
	if err != nil {
		panic("stdlib: the embedded runtime is missing: " + err.Error())
	}
	return sub
}

// SwiftBridgeUnit is the file of Runtime that converts between Vertex's
// String and Array and Swift's, for a target where Swift's runtime is
// part of the platform.
func SwiftBridgeUnit(target string) (string, bool) {
	if strings.HasSuffix(target, "-macos") {
		return "swift.cpp", true
	}
	return "", false
}

// The runtime's assembly, which is the two jobs C++ cannot do.
//
// One is entering a continuation: an async function is handed its context
// in the context register and a closure's captures in the self register,
// and nothing C++ says loads either. It also saves them, because an async
// function does not -- the context register is callee-saved under AAPCS64
// and is not under Swift's convention on top of it.
//
// The other is the shape of the runtime's own suspending primitives. They
// are async functions in the ABI sense, so they are entered with the
// context in X22 and their arguments from X0 -- and a C function reads
// its first argument from X0. Each is reached through a thunk that shifts
// its arguments up and puts the context in X0, then branches to the C. A
// branch and not a call, so that when the C returns it returns to the
// executor that entered the task.

// taskEnterARM64 is vertex_task_enter(code, async, self, a0, a1).
const taskEnterARM64 = "\n" +
	"\tsub sp, sp, #32\n" +
	"\tstp x20, x22, [sp, #0]\n" +
	"\tstr x30, [sp, #16]\n" +
	"\tmov x9, x0\n" +
	"\tmov x22, x1\n" +
	"\tmov x20, x2\n" +
	"\tmov x0, x3\n" +
	"\tmov x1, x4\n" +
	"\tblr x9\n" +
	"\tldp x20, x22, [sp, #0]\n" +
	"\tldr x30, [sp, #16]\n" +
	"\tadd sp, sp, #32\n" +
	"\tret\n"

// asyncEntries are the runtime's functions that are entered the way an
// async function is -- the context in X22 and arguments from X0 -- and
// how many arguments each takes besides the context. Each needs a thunk;
// see asyncThunk.
//
// All but the last are suspensions a program calls. vertex_task_done is
// the other kind: nothing calls it, it is what a task's root context
// names as its continuation, so a task's outermost return lands there.
var asyncEntries = []struct {
	name    string
	args    int
	suspend bool
}{
	{"vertex_task_yield", 0, true},
	{"vertex_task_sleep", 1, true},
	{"vertex_task_join", 1, true},
	{"vertex_task_wait_fd", 3, true},
	{"vertex_task_hop", 1, true},
	{"vertex_task_done", 1, false},
}

// Suspends reports whether a runtime call gives up the thread: it is an
// async function in the ABI sense, entered with a context and answering
// through the continuation that context names.
//
// A call to one is a suspension, so the compiler types it @async and the
// split treats it as it treats any other -- which is why there is no
// special case for the runtime anywhere in lowering.
func Suspends(symbol string) bool {
	for _, p := range asyncEntries {
		if p.name == symbol {
			return p.suspend
		}
	}
	return false
}

// asyncThunk is the assembly for one of them: its arguments up one, the
// context into the first, and on to the C behind it.
func asyncThunk(prefix, name string, args int) string {
	label := prefix + name
	out := "\t.text\n\t.globl " + label + "\n\t.p2align 2\n" + label + ":\n"
	// Downwards, so that moving X2 into X3 cannot overwrite an argument
	// that has not moved yet.
	for i := args; i >= 1; i-- {
		out += "\tmov x" + asmNum(i) + ", x" + asmNum(i-1) + "\n"
	}
	out += "\tmov x0, x22\n"
	out += "\tb " + label + "_at\n"
	return out
}

func asmNum(n int) string { return string(rune('0' + n)) }

// TaskAsm is the runtime's assembly for a target, as a module-scope block
// for the runtime's own object to carry, so that whatever links the
// runtime has it.
func TaskAsm(target string) (string, bool) {
	if !strings.HasPrefix(target, "arm64-") && !strings.HasPrefix(target, "aarch64-") {
		return "", false
	}
	prefix := ""
	if strings.HasSuffix(target, "-macos") {
		prefix = "_"
	}
	enter := prefix + "vertex_task_enter"
	out := "\t.text\n\t.globl " + enter + "\n\t.p2align 2\n" + enter + ":" + taskEnterARM64
	call := prefix + "vertex_witness_call"
	out += "\t.globl " + call + "\n\t.p2align 2\n" + call + ":" + witnessCallARM64
	call1 := prefix + "vertex_witness_call1"
	out += "\t.globl " + call1 + "\n\t.p2align 2\n" + call1 + ":" + witnessCall1ARM64
	static2 := prefix + "vertex_witness_call_static2"
	out += "\t.globl " + static2 + "\n\t.p2align 2\n" + static2 + ":" + witnessCallStatic2ARM64
	for _, p := range asyncEntries {
		out += asyncThunk(prefix, p.name, p.args)
	}
	if strings.HasSuffix(target, "-android") {
		// A zero record, so that every image has a vertex_proto for the
		// linker to bracket: see runtime/platform/android.h.
		out += "\t.section vertex_proto,\"aw\"\n\t.p2align 2\n\t.word 0\n\t.text\n"
	}
	return out, true
}

// RuntimeUnit is the file of Runtime to compile for a target, named the
// way the family names targets: arch-os.
//
// It is one translation unit per platform, which includes every part of
// the runtime and the one platform layer that target reaches its
// operating system through. One unit is one object, so the runtime is
// one input to a link however many files it is written in.
func RuntimeUnit(target string) (string, bool) {
	switch {
	case strings.HasSuffix(target, "-macos"):
		return "darwin.cpp", true
	case strings.HasSuffix(target, "-windows"):
		return "windows.cpp", true
	case strings.HasSuffix(target, "-android"):
		return "android.cpp", true
	}
	return "", false
}

// witnessCallARM64 is vertex_witness_call(fn, self, metadata, table): a
// witness called from C++, with the conformer's address in the self
// register and the metadata and table as its first two arguments, which is
// where a witness with no declared parameters finds them. What it answers
// stays in x0 and x1. x20 is callee-saved, so it is kept and put back.
const witnessCallARM64 = `
	stp x29, x30, [sp, #-32]!
	mov x29, sp
	str x20, [sp, #16]
	mov x16, x0
	mov x20, x1
	mov x0, x2
	mov x1, x3
	blr x16
	ldr x20, [sp, #16]
	ldp x29, x30, [sp], #32
	ret
`

// witnessCall1ARM64 is vertex_witness_call1(fn, self, arg, metadata, table):
// the one declared argument in x0, the conformer's address in the self
// register, and the metadata and table after the argument.
const witnessCall1ARM64 = `
	stp x29, x30, [sp, #-32]!
	mov x29, sp
	str x20, [sp, #16]
	mov x16, x0
	mov x20, x1
	mov x0, x2
	mov x1, x3
	mov x2, x4
	blr x16
	ldr x20, [sp, #16]
	ldp x29, x30, [sp], #32
	ret
`

// witnessCallStatic2ARM64 is vertex_witness_call_static2(fn, a, b,
// metadata, table): the operands' addresses in x0 and x1, the type in the
// self register -- a static witness's self is its metatype -- and the
// metadata and table after the operands.
const witnessCallStatic2ARM64 = `
	stp x29, x30, [sp, #-32]!
	mov x29, sp
	str x20, [sp, #16]
	mov x16, x0
	mov x20, x3
	mov x0, x1
	mov x1, x2
	mov x2, x3
	mov x3, x4
	blr x16
	ldr x20, [sp, #16]
	ldp x29, x30, [sp], #32
	ret
`
