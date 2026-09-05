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
// Closures, enums with payloads, existentials, generics and throwing
// wait on the same thing.
package gen
