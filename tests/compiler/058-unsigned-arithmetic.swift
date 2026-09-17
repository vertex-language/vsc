// Unsigned division and comparison do not treat the top bit as a sign.
func main() -> Int32 {
    let a: UInt32 = 4000000000
    let b: UInt32 = 2
    let half = a / b
    var score: Int32 = 0
    if half == 2000000000 { score = score + 1 }
    if a > 2147483647 { score = score + 2 }
    return score
}
