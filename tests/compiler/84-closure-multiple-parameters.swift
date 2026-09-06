// A closure of more than one argument.
func combine(_ f: (Int32, Int32) -> Int32, _ a: Int32, _ b: Int32) -> Int32 {
    return f(a, b)
}

func main() -> Int32 {
    let add: (Int32, Int32) -> Int32 = { a, b in a + b }
    let mul: (Int32, Int32) -> Int32 = { $0 * $1 }
    return combine(add, 20, 2) + combine(mul, 4, 5)
}
