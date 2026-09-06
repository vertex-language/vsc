// Written at the call, and with the shorthand names.
func apply(_ f: (Int32) -> Int32, _ x: Int32) -> Int32 { return f(x) }

func main() -> Int32 {
    let a = apply({ n in n + 1 }, 10)
    let square: (Int32) -> Int32 = { $0 * $0 }
    return a + square(5)
}
