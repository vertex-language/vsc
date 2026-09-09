// Package build takes a compiled module the rest of the way: VIR to
// an object file, and objects to a program that runs.
//
// It is a separate Go module, and the reason is the same one that made
// ir/lower a separate module. A backend is one architecture's worth of
// instruction selection and an object format's worth of writing, and
// pulling those in is a great deal for a program that only wants to
// typecheck something. Everything above this line depends on `ir` and
// nothing else; this is where the machine arrives.
//
// # The two steps
//
// Object lowers a VIR module and returns the bytes of an object file.
// Executable takes objects and returns the bytes of a program. Both
// take bytes and give bytes back, so nothing here writes a temporary
// file and an object never has to reach the filesystem to be linked.
//
// # No toolchain
//
// Neither step shells out. The instruction selector is ir/lower, the
// object writer is the architecture's obj package, and the linker is
// macho/link — vertex-language repositories, shared with vcc, each of
// which takes bytes and returns bytes. A machine with no cc, no as
// and no ld builds and runs a program with this package, which is the
// claim the compiler makes and is what build/link_test.go holds it
// to.
//
// What the platform still supplies is its own libraries. A hosted
// program links against libSystem, whose stub lives in the macOS SDK,
// and SDK() is the lookup for it. A freestanding link takes none of
// that and undertakes to define everything it names.
//
// # What is not here
//
// One target: aarch64 Mach-O. A backend and a writer exist for more,
// and each is a case in the switch that dispatches on the target;
// what is missing is the target table that names them, not the code
// they would call.
//
// A runtime, too. Nothing calls vertex_retain or vertex_release yet
// because nothing lowers a class, and when something does, the two
// functions are a VIR module this compiler emits for itself rather
// than C somebody has to compile first.
package build
