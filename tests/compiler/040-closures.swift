// A closure is a function written where it is used.
func apply(_ f: (Int32) -> Int32, _ x: Int32) -> Int32 { return f(x) }

func main() -> Int32 {
    let double: (Int32) -> Int32 = { n in n * 2 }
    return apply(double, 21)
}
