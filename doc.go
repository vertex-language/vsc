// Package vsc provides the core compilation pipeline:
//
//	source → scanner → parser → analyzer → internal/sil/gen → internal/sil/pass → lower → VIR
//
// It compiles source files up to machine VIR. Code generation and object file linking
// are handled by the build package.
package vsc
