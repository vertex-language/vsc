// A closure written inside another one.
func apply(_ f: (Int32) -> Int32, _ x: Int32) -> Int32 { return f(x) }

func main() -> Int32 {
    let outer: (Int32) -> Int32 = { n in
        let inner: (Int32) -> Int32 = { m in m * 2 }
        return inner(n) + 1
    }
    return apply(outer, 20) + 1
}
