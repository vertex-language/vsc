// A struct comes back from a call whole.
struct Pair {
    var a: Int32
    var b: Int32
}

func swapped(_ p: Pair) -> Pair {
    return Pair(a: p.b, b: p.a)
}

func main() -> Int32 {
    let p = swapped(Pair(a: 2, b: 40))
    return p.a + p.b * 1
}
