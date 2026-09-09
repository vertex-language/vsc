// Dispatching through an existential where the answer is not obvious
// from the call site.
//
// Every call below reads a row out of a table the value brought with
// it. What makes that worth testing separately is the ways of
// reaching one: a requirement a protocol inherits rather than
// declares, a method that takes an existential, and an existential
// standing next to the generic that would otherwise have handled it.
protocol Sized { func size() -> Int32 }
protocol Labelled: Sized { func label() -> Int32 }

struct Dot: Labelled {
    func size() -> Int32 { return 1 }
    func label() -> Int32 { return 100 }
}
struct Bar: Labelled {
    var length: Int32
    func size() -> Int32 { return length }
    func label() -> Int32 { return 200 }
}

// An inherited requirement, reached through the protocol that
// inherits it: the row is Labelled's, and Labelled promises what
// Sized promised.
func described(_ x: Labelled) -> Int32 { return x.label() + x.size() }

// A method whose parameter is an existential. The receiver goes last
// in the argument list and the boxing happens for the parameter
// before it, which are two separate orders and were once conflated.
struct Scale {
    var factor: Int32
    func of(_ x: Sized) -> Int32 { return x.size() * factor }
}

// The generic and the existential, side by side. The first is
// specialized per type argument; the second is one body for every
// type. They agree on the answer, which is the point.
func staticSize<T: Sized>(_ x: T) -> Int32 { return x.size() }
func dynamicSize(_ x: Sized) -> Int32 { return x.size() }

func main() -> Int32 {
    if described(Dot()) != 101 { return 91 }
    if described(Bar(length: 7)) != 207 { return 92 }
    if Scale(factor: 3).of(Bar(length: 5)) != 15 { return 93 }
    if staticSize(Bar(length: 9)) != dynamicSize(Bar(length: 9)) { return 94 }
    return described(Dot()) + Scale(factor: 2).of(Dot())
}
