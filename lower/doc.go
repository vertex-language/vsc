// Package lower translates VIL to VIR.
//
// This is the seam between the two halves of the compiler. Above it,
// everything is Swift: formal types, ownership, conventions, the
// vocabulary a diagnostic can be written in. Below it, everything is
// machine: registers of seven widths, memory reached through pointers,
// and no notion at all of what a value means. VIR is shared with vcc
// and v++, so nothing Swift-shaped may cross.
//
// The input must be a lowered module — ownership already erased by
// vil/pass. A module still in the ownership form is refused rather
// than lowered with its retains dropped on the floor, because dropping
// them silently is how a compiler learns to leak.
//
// # What a type becomes
//
// A Swift type is either held in a register or held in memory. Int is
// an i64, Bool an i1, a class reference a ptr. Int8 and Int16 are held
// in i32 registers, since §2 of the VIR spec makes those storage-only
// widths, and arithmetic on them is normalized back to the narrow
// range afterwards. Everything else — strings, tuples, structs with
// more than nothing in them, existentials — is an address, and this
// package will say so plainly rather than guess a layout.
//
// # What a symbol becomes
//
// The platform's prefix is applied here and nowhere else: "_" on
// Mach-O, nothing on ELF and COFF. It is Options.SymbolPrefix rather
// than something read off the target, because the mapping belongs to
// the language and not to the IR -- `nm` on a `swiftc -c` object
// shows _$s2m21fyS2iF and _main, so the underscore is on every symbol
// a Darwin object defines or names, mangled or not. Nothing below
// this package renames anything, which is what makes the rule
// checkable: one function applies it, and a lookup by VIL name
// happens before it does.
//
// # Structs
//
// A struct of one field is that field, and nothing is built or taken
// apart. Int is a struct around a Builtin.Int64 and Bool around a
// Builtin.Int1, so the rule is load-bearing before a program declares
// a wrapper of its own; a declared one lowers the same way and costs
// nothing. It holds in three places that have to agree — the `struct`
// instruction, `struct_extract`, and the machine type a parameter or
// a result is passed by — because a value that lowers inside a
// function and has no type to cross a call boundary by is a value
// that only half exists.
//
// More than one field is more than one register: one VIL value
// standing for several VIR ones, which is the shape an
// overflow-reporting builtin already produces. Nothing is emitted for
// building one or for reading a field back out — the scalars were
// computed where they were written, and the struct is the list of
// them, so `struct` records the list and `struct_extract` takes a
// window onto it.
//
// A struct inside a struct is the same list with the inner one's
// scalars spliced in where it sat, at its own offsets plus where it
// sits — leaves.go answers that once, and the four places a struct is
// taken apart all ask it rather than each walking the fields. That is
// what makes an inner struct extractable whole for nothing: its
// scalars are already there, and the answer is which of them.
//
// Crossing a call boundary is where the value has to be whole, and
// that is an ABI question. Swift's answer, read off what `swiftc -O`
// emits for AArch64 rather than guessed at: a struct of at most four
// words is passed in that many registers, and a wider one by address.
// The registers are the struct's memory image cut into words, not one
// per field — `struct { a: Int32; b: Bool }` arrives in a single
// register and Swift reads the flag out of bit 32, where the layout
// put it. abi.go does that packing, and does it from Offsetof so that
// a struct written through a pointer and one read out of registers
// cannot disagree about where a field is.
//
// The agreement with C holds to sixteen bytes and then stops: C
// passes a larger composite by address and Swift passes up to
// thirty-two in registers. This follows Swift, so a struct between
// those sizes is not C-callable, and build/run_test.go's interop
// tests stay under the limit where the two conventions coincide.
//
// In memory a struct is written and read one field at a time, each at
// the offset the layout gives it. Nothing puts the whole value
// anywhere — there is no register wide enough, and going through the
// packed word form would write padding the layout does not have — so
// the two representations meet only through Offsetof, which is what
// keeps them agreeing about where a field is.
//
// Past four words a struct is refused rather than passed in a shape
// invented here — an ABI two compilers disagree about is a program
// that links and returns nonsense. A store of one, and a field that
// is itself a wide struct, are refused for the same reason: both need
// the memory layout that going by address would also need.
//
// # An enum, and the branch on one
//
// An enum with no associated values is its tag and nothing else, so
// it lowers to an integer as wide as the tag needs — one byte up to
// 256 cases, which is what types.Sizeof says and what Swift lays out.
// A case is the constant of its position among the ones the type
// declares, and switch_enum is a jump table indexed by that position:
// the tags are 0, 1, 2 … by construction, which is the one shape a
// table is always right for.
//
// The table is dense whether or not the switch names every case. A
// case the switch left out goes where anything unnamed goes, which is
// the default edge — vil/gen supplies one even where the source named
// every case, because a table has to say where an out-of-range
// selector lands and a machine has no notion of the type saying there
// are none.
//
// An enum with a payload is refused, here and in gen both: its layout
// is the payload beside the tag, and nothing computes that yet.
//
// # Where a frame slot goes
//
// VIR admits a frame allocation in the entry block and nowhere else,
// and SIL writes alloc_stack where the variable was declared — which
// for a `var` inside a loop is a block that runs many times. Every
// slot the function needs is therefore reserved up front, before any
// block is walked, and the alloc_stack itself lowers to nothing.
//
// A scalar local declared in a loop is one slot written afresh on
// each pass rather than a new slot per iteration, which is what the
// program says: VIL stores the initializer where the declaration was.
// What is given up is a shorter lifetime — dealloc_stack becomes
// nothing, and two variables in disjoint scopes get two slots. That
// is a frame larger than it needs to be and never a wrong program.
//
// # Classes
//
// A class value is a reference: one pointer, whatever the instance
// holds. Making one is a call to the runtime's allocator, and the
// size asked for is the instance's — the stored properties laid out —
// and not the value's. The difference is the whole of it: Sizeof of a
// class is one word, which is right for a register and would give
// every instance eight bytes to hold whatever it declared.
//
// A property is reached by arithmetic on the reference, past a
// two-word header the runtime owns. That number is the one thing this
// package and runtime/ have to agree about, and they agree by both
// naming the same constant.
//
// # What it refuses
//
// A great deal, at present, and each refusal names the instruction it
// could not translate. That is the deliberate shape: the alternative
// to refusing is emitting something that runs and is wrong, and a
// wrong program that runs is worse than one that does not compile.
package lower
