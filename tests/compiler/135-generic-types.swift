// A generic type is the type its arguments make of it: Box<Int32> is
// a struct holding an Int32, which is what makes every question about
// its size, its triviality and its register answerable.
struct Box<T> { var value: T }
struct Pair<A, B> { var first: A; var second: B }

func unwrap<T>(_ b: Box<T>) -> T { return b.value }

func main() -> Int32 {
    let a: Int32 = 20
    let b: Int64 = 22

    // The same generic type at two different arguments, which are two
    // different types with two different layouts.
    let small = Box(value: a)
    let large = Box(value: b)
    if small.value != 20 { return 91 }
    if large.value != 22 { return 92 }

    // Two parameters, and the same type instantiated both ways round.
    let p = Pair(first: a, second: b)
    let q = Pair(first: b, second: a)
    if p.first != 20 { return 93 }
    if q.second != 20 { return 94 }
    if p.second != q.first { return 95 }

    // One generic type inside another.
    let nested = Box(value: Pair(first: a, second: a))
    if nested.value.first + nested.value.second != 40 { return 96 }

    return unwrap(small) + Int32(unwrap(large))
}
