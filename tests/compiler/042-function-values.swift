// A declared function is a value of its type.
func triple(_ n: Int32) -> Int32 { return n * 3 }
func twice(_ f: (Int32) -> Int32, _ x: Int32) -> Int32 { return f(f(x)) }

func main() -> Int32 {
    let f: (Int32) -> Int32 = triple
    return twice(f, 2) - twice(triple, 1) * 0 + f(0)
}
