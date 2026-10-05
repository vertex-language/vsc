// Package gen lowers a type-checked AST into raw SIL (SILGen).
//
// # Ownership and Scopes
//
// Every copy, destroy, and borrow is explicitly emitted during generation:
//   - Class reference bindings emit copy_value on initialization and destroy_value at scope exit.
//   - Member reads borrow base objects for the duration of the read.
//   - A scoped cleanup stack manages cleanups in reverse order and unwinds on function exit.
//
// # Lowered Language Features
//
// The generator translates:
//   - Functions, parameters, return values, and local let/var bindings.
//   - Member reads and writes on structs and classes.
//   - Control flow: if, guard, while, repeat-while, for-in (ranges), switch, break, continue, and return.
//   - Short-circuit expressions (&&, ||, ? :) with phi/join block arguments.
//   - Static and dynamic method dispatch: direct calls, vtable lookups for classes, and witness tables for existentials (any P).
//   - Struct and class initializers (memberwise and default initializers).
//   - Non-capturing closures and function values.
//   - Generics via monomorphization.
//   - Tuples, inout parameters, optionals, enum payload handling, default arguments, and computed properties.
//   - Numeric conversions with overflow/truncation checks.
//
// Features not yet supported or requiring full standard library definitions
// return explicit diagnostic errors rather than being silently dropped.
package gen
