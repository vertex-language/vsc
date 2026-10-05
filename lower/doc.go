// Package lower translates SIL to VIR.
//
// The input must be a lowered SIL module with ownership erased.
// Swift types are mapped to VIR primitive registers or memory addresses:
//   - Primitive integers and booleans map to i1, i8, i16, i32, i64.
//   - Class instances and function pointers map to ptr.
//   - Small structs (up to 4 words on 64-bit targets) are passed in registers.
//   - Larger composites and existentials are passed indirectly via pointers.
//   - Enums without payload lower to their integer tag representation.
package lower
