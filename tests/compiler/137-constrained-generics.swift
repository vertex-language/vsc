// A generic constrained by a protocol, resolved where the type is
// known.
//
// swiftc passes a witness table at run time and dispatches the
// constrained call through it. Monomorphisation makes that a lookup
// instead: the body is lowered once per type argument, so by the time
// `x.value()` is reached the type is known and the implementation can
// be named. There is no table to index because there is nothing left
// to decide.
//
// That is also the limit of it. It works because the type is known,
// and the type is known because the body was specialized -- an
// existential, whose type arrives with the value, has neither.
protocol Valued { func value() -> Int32 }
protocol Named: Valued { func tag() -> Int32 }

struct Small: Valued { func value() -> Int32 { return 5 } }
struct Big: Named {
    func value() -> Int32 { return 30 }
    func tag() -> Int32 { return 7 }
}

// One constraint.
func doubled<T: Valued>(_ x: T) -> Int32 { return x.value() * 2 }

// An inherited requirement, reached through the protocol that inherits it.
func both<T: Named>(_ x: T) -> Int32 { return x.value() + x.tag() }

// Two parameters, each constrained.
func mix<A: Valued, B: Named>(_ a: A, _ b: B) -> Int32 {
    return a.value() + b.tag()
}

// The same generic at two different conforming types.
func main() -> Int32 {
    if doubled(Small()) != 10 { return 91 }
    if doubled(Big()) != 60 { return 92 }
    if both(Big()) != 37 { return 93 }
    if mix(Small(), Big()) != 12 { return 94 }
    return doubled(Small()) + both(Big()) - 5
}
