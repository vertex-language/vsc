// & binds tighter than ==, | and ^ tighter than comparison.
func main() -> Int32 {
    let a: Int32 = 0b1100
    let b: Int32 = 0b1010
    var n: Int32 = 0
    if a & b == 8 { n += 1 }
    if a | b == 14 { n += 2 }
    if a ^ b == 6 { n += 4 }
    if a & b | 1 == 9 { n += 8 }
    if a << 1 & 24 == 24 { n += 16 }
    return n
}
