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
// the container's — vertex-language repositories, shared with vcc,
// each of which takes bytes and returns bytes. A machine with no cc,
// no as, no ld and no link.exe builds and runs a program with this
// package, which is the claim the compiler makes and is what
// build/link_test.go holds it to on every target.
//
// What the platform still supplies is its own libraries, and build/
// sysroot is where they are found. A hosted macOS program links
// against libSystem, whose stub lives in the SDK; a hosted Windows
// program links against the static MSVC runtime, which is two
// installations found by a walk rather than one path. A freestanding
// link takes neither and undertakes to define everything it names.
//
// # The targets
//
// Two: aarch64 Mach-O and x86-64 PE. Each is a case in the switch
// that dispatches on the target's use path, in Object and again in
// Executable, and the two switches are the whole of what a target
// costs here — the backend, the writer and the linker are all
// somebody else's package.
//
// A backend and a writer exist for more than these two. What a third
// row would need is the same three lines and a way to link what comes
// out; a row that compiles but cannot be linked is a promise this
// package does not keep, which is why vsc/target.go has no such row.
//
// # What is not here
//
// Not every program builds for every target, and the one difference
// today is not the backend's. `print`, and the array and Any
// machinery under it, lower to calls into libswiftCore, which is part
// of the system on macOS and does not exist on Windows. Such a
// program is refused for x86_64-windows, and is refused in lowering
// rather than in the link: the Microsoft convention returns one
// value, and the two-word return those calls want is an sret the
// amd64 backend has not written yet. Either half alone would be
// enough to stop it.
//
// The Vertex runtime is not in that position. vertex_alloc,
// vertex_retain and vertex_release are a VIR module this compiler
// emits for itself — see Runtime — so they go through the same
// instruction selection and the same object writer as the program
// that calls them, and a target that can compile a program can build
// the runtime for it by construction.
package build
