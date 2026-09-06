// Three words, still passed in registers.
struct Triple {
    var a: Int
    var b: Int
    var c: Int
}

func total(_ t: Triple) -> Int { return t.a + t.b + t.c }

func main() -> Int32 {
    return Int32(total(Triple(a: 10, b: 20, c: 12)))
}
