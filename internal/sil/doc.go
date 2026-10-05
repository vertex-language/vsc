// Package sil implements the Swift Intermediate Language (SIL) ownership IR.
//
// SIL bridges semantic analysis and machine code generation (VIR/lowering).
// It models ownership semantics (OSSA), borrow scopes, lifetime analysis,
// and ARC operations.
//
// Modules transition between two stages:
//   - StageRaw: Generated directly by sil/gen before mandatory diagnostic and optimization passes.
//   - StageCanonical: Output of sil/pass and verified by sil/verify; consumed by lower.
//
// Core OSSA invariants:
//  1. An owned value must be consumed exactly once along every execution path.
//  2. A guaranteed value may only be used within the borrow scope introducing it.
//
// Modules consist of Funcs, Globals, VTables, and WitnessTables. Functions contain
// basic blocks accepting arguments in SSA form.
package sil
