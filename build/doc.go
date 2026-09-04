// Package build takes a compiled module the rest of the way: VIR to a
// finished object file.
//
// It is a separate Go module, and the reason is the same one that made
// ir/lower a separate module. A backend is one architecture's worth of
// instruction selection and an object format's worth of writing, and
// pulling those in is a great deal for a program that only wants to
// typecheck something. Everything above this line depends on `ir` and
// nothing else; this is where the machine arrives.
//
// What it does not do is link. Producing an executable means deciding
// where a runtime comes from, and that is vcc's business: it compiles
// the C the runtime is written in and links the result. This package
// stops at the object file, which is the last artifact that is
// entirely the compiler's own.
package build
