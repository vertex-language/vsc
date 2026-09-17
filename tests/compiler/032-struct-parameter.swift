// A struct crosses a call as a value: the callee sees a copy.
struct Pair {
    var a: Int32
    var b: Int32
}

func sum(_ p: Pair) -> Int32 { return p.a + p.b }

func main() -> Int32 {
    let p = Pair(a: 12, b: 30)
    return sum(p)
}
