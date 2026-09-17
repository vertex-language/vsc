// A pointer moved by an Int moves by that many of its elements, and a
// raw pointer by that many bytes: p + 2, q - 2, raw + 8, and 2 + p.
func check(_ p: UnsafeMutablePointer<Int32>) -> Int32 {
    var score: Int32 = 0
    let q = p + 2
    if q - 2 == p {
        score += 1
    }
    let raw = UnsafeRawPointer(p)
    if raw + 8 == UnsafeRawPointer(q) {
        score += 10
    }
    let step = 4
    if UnsafeRawPointer(p + 1) == raw + step {
        score += 100
    }
    if 2 + p == q {
        score += 1000
    }
    return score
}

func main() -> Int32 {
    var x: Int32 = 7
    return check(&x) % 251
}
