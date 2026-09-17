// A method of a generic type may have generic parameters of its own,
// inferred from what a call passes: map<U> on a Wrapper<T> takes U from
// what its closure returns. A generic function learns a parameter the same
// way, and a closure passed before the argument that fixes its types is
// read once that argument has.
struct Wrapper<T> {
    var value: T

    func map<U>(_ f: (T) -> U) -> Wrapper<U> {
        return Wrapper<U>(value: f(value))
    }

    func pair<U>(_ other: U) -> (T, U) {
        return (value, other)
    }
}

func apply<T, U>(_ x: T, _ f: (T) -> U) -> U {
    return f(x)
}

func twice<T>(_ f: (T) -> T, _ x: T) -> T {
    return f(f(x))
}

func main() -> Int32 {
    let w = Wrapper(value: 3)
    let scaled = w.map { n in n * 5 }
    let named = w.map { n in "item \(n) of a list long enough to live on the heap" }
    let both = w.pair("x")
    var total = scaled.value + named.value.count + both.0 + both.1.count
    total += apply(4) { n in n + 10 }
    total += apply("abc") { s in s.count }
    let base = 2
    total += twice({ $0 + base }, 1)
    return Int32(total % 251)
}
