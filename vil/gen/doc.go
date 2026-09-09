// Package gen lowers a checked tree into raw VIL.
//
// SILGen's job, and SILGen's name for it. The analyzer decided what
// the program means; this decides what it does — which accessor a
// property reference calls, where a temporary lives, when a value is
// copied and where its lifetime ends. It rejects nothing: a program
// that reaches here was already found legal, and what this produces
// is raw VIL for vil/pass to check and vil/lower to translate.
//
// # Ownership is emitted, not inferred
//
// Every copy and every destroy is written down here. A `let` that
// binds a class reference copies it and destroys it where its scope
// ends; a member read borrows the base for exactly as long as the
// read takes. That is what makes the output verifiable the moment it
// exists: vil/verify checks the two rules against what this emitted,
// rather than against what a later pass hopes to work out.
//
// The mechanism is a stack of scopes. Entering one pushes; leaving
// one emits the cleanups it collected, in reverse; a return unwinds
// every scope on the way out. It is SILGen's cleanup stack, smaller.
//
// # What is lowered
//
// Functions, their parameters and results. Local `let` and `var`
// bindings. Member reads and writes on structs and classes.
// Assignment. The control flow that is branches and blocks: `if`,
// `else`, `guard`, `while`, `repeat`-`while`, `break` and `continue`
// — labelled or not — and `return`. The three forms that produce a
// value on two paths and join them, which are `&&`, `||` and the
// conditional operator: each evaluates only the arm it needs, and
// each hands the answer to the join as a block argument, because an
// SSA value is usable only where it dominates its readers. Calls to
// functions the checker resolved.
//
// Methods, on a struct or on a final class: an ordinary function with
// the receiver as its last parameter, called through a function_ref.
// Static dispatch only — a method that may be overridden goes through
// the object's table, and neither the table nor inheritance is
// modelled. A bare name in a method body that is a stored property or
// another method of the same type is self's.
//
// Making an instance. A struct with no initializer of its own is
// made by the memberwise one, whose body is a `struct` instruction
// over its arguments; a class with none is made by the one it gets
// for free, which allocates and stores each property's initial
// value. Both are emitted inlined rather than as a call, because the
// initializer is declared nowhere and a call to a function that does
// not exist is worse than the body of the one that would have run.
// An initializer a type declares itself is refused.
//
// Writing through a name: a `var`, a class's property through the
// reference it is inside, and a struct's property through the address
// of the struct — the base of a struct member is an address for the
// same reason the member is, because reading the struct out into a
// value first would write into the copy.
//
// Switch, in the two shapes SILGen has and picked between the same
// way. A subject that is an enum branches on its tag with
// switch_enum; anything else is a chain of comparisons, each case
// tested in turn and a failed test falling into the next, which is
// what Swift's pattern match over Equatable is rather than a jump
// table. A case body does not fall into the next one — Swift breaks
// implicitly at its end — and `break` inside a switch leaves the
// switch where `continue` leaves the enclosing loop, which is the one
// place the two keywords name different statements.
//
// The block after a switch is made only when something branches to
// it. Where every case returns there is nothing after the statement,
// and a block with no predecessors is one the verifier rejects.
// Swift reaches the same place differently: SILGen gives a function
// one epilog block that every return branches to with its value, so
// the continuation always has a predecessor. This returns from each
// case directly, which is simpler and needs the block to be
// conditional instead.
//
// An enum case with no associated value, written out as `Color.red`
// or with the leading dot the context resolves. A case that carries
// one is refused: the payload's layout is not computed anywhere yet.
//
// For-in, over a range of integers and nothing else. This is the one
// construct that does not follow SILGen: the language defines a for-in
// as makeIterator() and next() until it returns none, and every piece
// of that is generic stdlib -- Range, Collection, IndexingIterator,
// Optional -- so lowering the desugaring would mean calling functions
// declared nowhere. What is emitted is what `swiftc -O` produces once
// it has specialized all of it away, which is the same program: two
// bounds evaluated once, an index, a comparison, and Swift's own trap
// for a range that runs backwards. A for-in over anything else is
// refused by name.
//
// Closures that capture nothing, and a declared function used as a
// value. A closure body is a function of its own -- SILGen emits one
// as `sil private` and refers to it -- so that is what this emits,
// and the expression becomes a function_ref given the shape of a
// function value. A closure that captures is refused by name: the
// captures would be trailing parameters bound by partial_apply, and
// what that needs beyond a bigger case here is a heap context, a
// reference count on it, and a forwarder for the arity it does not
// match. See closure.go.
//
// Classes with inheritance, dispatched through a table. A method call
// is a class_method when the receiver's class has a superclass or is
// one, and a function_ref otherwise -- a class in neither group has a
// single implementation and always will. The tables are in vtable.go,
// and the invariant that makes them work is that every table down a
// chain repeats its base's rows in its base's order.
//
// Generics, by monomorphisation. A generic function has no body until
// something says what its type parameters are; the call that says so
// gets a copy of the body lowered for those types, which is what
// swiftc -O's specializer leaves. A call through a protocol
// constraint is then a lookup rather than a dispatch, because by the
// time the body is lowered the type is known. See generic.go.
//
// An associated type needs nothing of its own here, which is the
// point of monomorphisation: by the time a body is lowered, C is a
// concrete type and `C.Item` is what that type chose, so there is
// nothing left to look up. Only an existential over such a protocol
// is refused -- the choice arrives with the value, and an existential
// carries no metadata to carry it in.
//
// Existentials, dispatched through a witness table. `any P` is the
// case monomorphisation cannot reach: the value's type arrives with
// the value, so the implementation is read out of a table the value
// carries. One table per conformance, one row per requirement, each
// row a thunk that takes the receiver by address. See witness.go,
// which also says what an existential here may hold and why.
//
// Tuples, built from their elements the way SILGen builds one. What
// their bytes are is lower's question -- a tuple is the struct with
// the same elements -- and so is the one place the two differ: a
// tuple parameter is flattened into the parameter list, one register
// per element, while a struct parameter is packed into words.
//
// Calls that may fail, as far as `try?` goes. Swift carries the
// failure beside the result, in a register the caller clears before
// the call and reads after -- so one call has two edges out of it,
// which is what `try_apply` says. `try?` discards the error and
// produces an optional, which needs no error value at all; anything
// that looks at the error needs `any Error`, and a `throw` needs one
// to make. Both are refused. See throwing.go.
//
// `inout`, which hands over the caller's storage rather than a copy
// of what is in it: `&a` is an address opened under a modify access,
// and the callee's parameter is that address. It is also the one
// parameter a body may write to, which is what makes it worth having.
//
// Enums whose cases carry values. The cases share the space the
// payload goes in, so an enum is the largest of them and a byte to
// say which is there -- and the tags are the cases that carry
// something first, in declaration order, then the ones that carry
// nothing. What crosses a call is the bytes rather than the fields,
// because unlike a struct's the fields overlap. See lower/enum.go.
//
// Default arguments, filled in at the call, which is where Swift
// evaluates one. A call that leaves a parameter out is not a call
// with fewer arguments: the caller supplies the rest, and one that
// passed only what was written left the callee reading a register
// nobody wrote. See defaults.go.
//
// Members of the type rather than of an instance: a static method has
// no receiver to pass -- a struct's metatype is thin, which is no
// register -- and its symbol is the method's with Z after it. A
// static stored property has storage of its own, reached through an
// accessor rather than at an offset, and one declared here is refused
// until this compiler has globals to give it.
//
// Computed properties, which are functions that look like fields: a
// read is a call to the getter, and a type that declares one has that
// getter emitted. See computed.go.
//
// Optionals, which are enums and are written as ones. `let a: Int32?
// = 7` is `.some(7)`, and nothing in the source says so -- Swift
// injects it on the way in and so does this, at a binding and at a
// call. Reading one back is `switch_enum` on which case it holds,
// with the payload arriving as the arm's block argument, which is
// what `if let` is. See optional.go, and lower's optionalImage for
// what the bytes are.
//
// Numeric conversions. `Int32(n)` names a type and is a conversion
// rather than a construction: a range check and a change of width,
// which is what swiftc -O leaves once it has inlined the initializer
// the standard library declares. A value the destination cannot
// represent traps, in Swift's own words. See convert.go.
//
// Two rules of the language are checked here because nothing before
// this models them: a `break` or `continue` has to be inside a loop
// it names, and the body of a `guard` may not fall through. Both are
// errors about the program rather than refusals about the compiler,
// and they read that way.
//
// Everything else is refused by name and said out loud, which is a
// rule rather than a courtesy. An expression that produced no value
// and no diagnostic used to take its statement with it, and a
// statement kind with no case here used to vanish entirely — a
// `while` that never ran, in a program that compiled, linked and
// gave an answer. Both are why every path out of this package that
// lowers nothing reports it.
//
// What is not, and why: anything that needs a standard library. An
// integer literal in Swift is a call to `Int.init(_builtinIntegerLiteral:)`,
// and until core/ declares that, this emits the builtin literal
// directly and the output is one apply short of what swiftc prints.
// Closures, enums with payloads and throwing wait on the same thing.
package gen
