// Generics, by monomorphisation: a generic function is lowered once
// per set of type arguments some call inferred, with the body walked
// as though the parameters had been written out.
//
// swiftc emits the generic function once instead, and passes the
// types at run time as metadata. What its optimizer then does to the
// common case -- specialize per call site -- is what this does to
// every case, which is why generics stop at the module boundary here.
struct Pair<A, B> { var first: A; var second: B }
struct Box<T> { var value: T }

func unwrap<T>(_ b: Box<T>) -> T { return b.value }
func swapped<A, B>(_ p: Pair<A, B>) -> Pair<B, A> {
    return Pair(first: p.second, second: p.first)
}
func twice<T>(_ f: (T) -> T, _ x: T) -> T { return f(f(x)) }

func main() -> Int32 {
    let a: Int32 = 20
    let b: Int64 = 22

    // Two type parameters, and instantiated both ways round.
    let p = Pair(first: a, second: b)
    let q = swapped(p)
    if Int32(q.first) != 22 { return 91 }
    if q.second != 20 { return 92 }

    // A generic function taking a generic type.
    if unwrap(Box(value: a)) != 20 { return 93 }
    if Int32(unwrap(Box(value: b))) != 22 { return 94 }

    // Nested instantiation.
    let nested = Box(value: Box(value: a))
    if unwrap(unwrap(nested)) != 20 { return 95 }

    // A generic function taking a function.
    let inc: (Int32) -> Int32 = { $0 + 1 }
    if twice(inc, 40) != 42 { return 96 }

    return a + Int32(b)
}
