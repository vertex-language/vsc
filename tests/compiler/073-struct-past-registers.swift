// A struct argument after the registers are used up.
struct Pair {
    var a: Int32
    var b: Int32
}

func take(_ p1: Int32, _ p2: Int32, _ p3: Int32, _ p4: Int32,
          _ p5: Int32, _ p6: Int32, _ p7: Int32, _ pair: Pair) -> Int32 {
    return p1 + p2 + p3 + p4 + p5 + p6 + p7 + pair.a + pair.b
}

func main() -> Int32 {
    return take(1, 2, 3, 4, 5, 6, 7, Pair(a: 10, b: 4))
}
