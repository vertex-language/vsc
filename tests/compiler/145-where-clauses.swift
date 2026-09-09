// The constraints the angle brackets cannot carry.
//
//     func doubled<C: Container>(_ c: C) -> Int32 where C.Item == Int32
//
// `<C: Container>` says what C is. It cannot say anything about
// C.Item, because C.Item is not a parameter -- and that is what a
// where clause is for. A same-type requirement makes C.Item that
// type for as long as the clause is in force, so the body may add to
// it; a conformance requirement says what it satisfies, which is the
// same thing `associatedtype Item: Number` says about every
// conformer rather than about this call's.
protocol Container {
    associatedtype Item
    func get() -> Item
}
protocol Number { func value() -> Int32 }

struct Ints: Container { func get() -> Int32 { return 21 } }
struct Flags: Container { func get() -> Bool { return true } }

struct N: Number {
    var n: Int32
    func value() -> Int32 { return n }
}
struct Ns: Container { func get() -> N { return N(n: 5) } }

// Inside this body C.Item is Int32, so arithmetic on it checks.
func doubled<C: Container>(_ c: C) -> Int32 where C.Item == Int32 {
    return c.get() * 2
}

// And here it is something with a value(), whatever it is.
func valued<C: Container>(_ c: C) -> Int32 where C.Item: Number {
    return c.get().value()
}

// The same protocol with neither clause: nothing is known about
// C.Item, so nothing is done with it.
func has<C: Container>(_ c: C) -> Int32 { return 1 }

func main() -> Int32 {
    if doubled(Ints()) != 42 { return 91 }
    if valued(Ns()) != 5 { return 92 }
    if has(Flags()) != 1 { return 93 }
    return doubled(Ints()) + valued(Ns()) - 5
}
