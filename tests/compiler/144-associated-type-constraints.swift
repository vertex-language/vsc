// What the conformer's choice has to satisfy.
//
//     associatedtype Item: Number
//
// A protocol that says only `associatedtype Item` has said nothing
// about Item, so a body holding one can do nothing with it. A
// constraint is what makes it useful: whatever Item turns out to be,
// it has a value(), and `c.get().value()` checks before anything
// knows which type it is.
//
// A conformer whose choice does not satisfy the constraint does not
// conform, and is told so where it is declared rather than where the
// requirement is next used.
protocol Number { func value() -> Int32 }

protocol Container {
    associatedtype Item: Number
    func get() -> Item
}

// An associated type declared by an inherited protocol is the
// inheriting protocol's too.
protocol Counted: Container {
    func count() -> Int32
}

struct N: Number {
    var n: Int32
    func value() -> Int32 { return n }
}
struct Doubling: Number {
    var n: Int32
    func value() -> Int32 { return n * 2 }
}

struct Ns: Counted {
    func get() -> N { return N(n: 5) }
    func count() -> Int32 { return 3 }
}
struct Ds: Container {
    func get() -> Doubling { return Doubling(n: 8) }
}

// The constraint is what lets this call value() at all.
func valueOf<C: Container>(_ c: C) -> Int32 { return c.get().value() }

// Reached through the protocol that inherits it: Counted promises
// what Container promised.
func summed<C: Counted>(_ c: C) -> Int32 { return c.get().value() + c.count() }

func main() -> Int32 {
    if valueOf(Ns()) != 5 { return 91 }
    if valueOf(Ds()) != 16 { return 92 }
    if summed(Ns()) != 8 { return 93 }
    return valueOf(Ns()) + valueOf(Ds()) + summed(Ns()) - 21
}
