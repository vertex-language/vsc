// Package vsc is the compiler: the phases composed, and nothing of
// its own.
//
// Everything here is a call into one of the packages below it, in
// order. What this package contributes is the order, the places a
// diagnostic can stop it, and a name for the thing that comes out of
// each phase -- so that a caller wanting the AST, the ownership IR, or
// the machine IR asks for it rather than reproducing the sequence.
//
//	source → scanner → parser → analyzer → vil/gen → vil/pass → lower → VIR
//
// It stops at VIR. Turning that into an object file needs a backend
// for one architecture and a writer for one object format, and those
// are a heavier dependency than a package that only wants to typecheck
// something should carry -- so they live in ./build, which is its own
// module for exactly that reason.
package vsc
