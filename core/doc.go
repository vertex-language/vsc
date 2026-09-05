// Package core is the built-in module: what every program can see
// without importing anything.
//
// It is two things about the same declarations, from the two ends
// they are needed at.
//
// For the front end it is source. core.swift declares the operators
// on the primitive types, and the analyzer checks it the way it
// checks any other file — so `1 + 2` resolves to a declaration, and a
// call to one is a call rather than a rule buried in the checker.
//
// For the back end it is layout and implementation. An operator's
// declaration has no body, because its body is a machine instruction:
// Int's `+` is an add, and Layout and Lower below say which. Swift's
// own `+` is the same arrangement written differently — a
// @_transparent wrapper the first mandatory pass inlines away,
// leaving the builtin behind.
//
// # What is not here yet
//
// The primitive types themselves. `Int` is a types.Basic in the
// universe rather than a struct declared here, which is where Swift
// puts it. Declaring it would mean the checker resolving `Int.max`
// and `1.description` the ordinary way, and it would mean this
// package growing a body for each of them; both wait on a reason.
// Layout is the part the IR needs meanwhile, and it says what Swift's
// declaration says: an Int is a Builtin.Int64 in a field named
// _value.
//
// Precedence is the other absence. The groups and the operator-to-
// group table are built into the analyzer rather than declared here,
// which is a second source of truth for something Swift keeps in one
// place. Moving them is a change to a package that is finished and
// working, and it is worth doing when core declares the types too.
package core
