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
// # What it refuses
//
// A great deal, at present, and each refusal names the instruction it
// could not translate. That is the deliberate shape: the alternative
// to refusing is emitting something that runs and is wrong, and a
// wrong program that runs is worse than one that does not compile.
package lower
