// The masking operators keep the low bits where the checked ones
// would trap, and the masking shifts take the count modulo the width.
func main() -> Int32 {
    var n: Int32 = 0
    let hi: Int32 = 2147483647
    let lo: Int32 = -2147483648
    if hi &+ 1 == lo { n += 1 }
    if lo &- 1 == hi { n += 2 }
    if hi &* 2 == -2 { n += 4 }
    let a: Int32 = 1
    if a &<< 33 == 2 { n += 8 }
    if a &<< 32 == 1 { n += 16 }
    let b: Int32 = -256
    if b &>> 33 == -128 { n += 32 }
    var c: Int32 = hi
    c &+= 1
    if c == lo { n += 64 }
    let d: UInt8 = 200
    if d &+ 100 == 44 { n += 128 }
    return n % 128
}
