// Four words is the last size passed in registers.
struct Quad {
    var a: Int
    var b: Int
    var c: Int
    var d: Int
}

func total(_ q: Quad) -> Int { return q.a + q.b + q.c + q.d }

func main() -> Int32 {
    return Int32(total(Quad(a: 1, b: 2, c: 3, d: 36)))
}
