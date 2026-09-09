// Every width compares and converts.
func main() -> Int32 {
    let a: Int8 = -128
    let b: Int16 = 300
    let c: UInt8 = 255
    let d: UInt32 = 70000

    var score: Int32 = 0
    if a < 0 { score = score + 1 }
    if b > 0 { score = score + 2 }
    if c > 200 { score = score + 4 }
    if d > 65535 { score = score + 8 }
    if Int32(a) == -128 { score = score + 16 }
    if Int32(c) == 255 { score = score + 32 }
    return score
}
