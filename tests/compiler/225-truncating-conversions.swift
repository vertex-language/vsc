// T(truncatingIfNeeded: x) keeps x's low bits when T is narrower, and
// extends them by x's sign -- or with zeros, for an unsigned x -- when T
// is wider. It checks nothing.
func main() -> Int32 {
    let big = 70000
    let a = UInt16(truncatingIfNeeded: big)
    let b = Int8(truncatingIfNeeded: 300)
    let raw: UInt8 = 200
    let c = CChar(truncatingIfNeeded: raw)
    let neg: Int32 = -5
    let d = Int(truncatingIfNeeded: neg)
    let minusOne: Int8 = -1
    let e = UInt32(truncatingIfNeeded: minusOne)
    let f = UInt64(truncatingIfNeeded: raw)
    let same = Int32(truncatingIfNeeded: neg)
    var total = Int(a) + Int(b) + Int(c) + d
    total += Int(e % 1000) + Int(f) + Int(same)
    return Int32(truncatingIfNeeded: total)
}
