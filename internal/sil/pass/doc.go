// Package pass transforms a SIL module between stages of the compilation pipeline.
//
// The pipeline advances modules through mandatory lowering passes:
//
//	raw ──▶ mandatory passes ──▶ canonical ──▶ LowerOwnership ──▶ lowered
//
// Mandatory passes include allocbox-to-stack promotion and definite initialization
// lowering. LowerOwnership lowers OSSA ownership instructions to explicit retain/release
// operations before code generation.
package pass
