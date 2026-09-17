// Package verify validates structural, SSA dominance, and OSSA ownership invariants
// for SIL modules.
//
// Verification checks:
//   - Structural validity: CFG well-formedness, valid terminators, block reachability,
//     branch argument compatibility, and function type consistency.
//   - Dominance: Every value definition dominates all of its uses.
//   - OSSA Rule 1: Owned values must be consumed exactly once along every execution path.
//   - OSSA Rule 2: Guaranteed values must only be used within their borrow scopes.
package verify
