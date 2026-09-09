// `Int32(d)` truncates toward zero and traps where the value has no
// place in the destination. The bounds are checked against the
// source's own type, which is what makes the test exact: a signed
// destination of n bits holds [-2^(n-1), 2^(n-1)), and both powers of
// two are exactly representable in binary floating point.
//
// Truncation, not rounding: 2.9 is 2 and -2.9 is -2.
func toInt32(_ d: Double) -> Int32 {
    return Int32(d)
}

func toInt64(_ d: Double) -> Int64 {
    return Int64(d)
}

func fromFloat(_ f: Float) -> Int32 {
    return Int32(f)
}

func unsignedFromDouble(_ d: Double) -> UInt32 {
    return UInt32(d)
}

func main() -> Int32 {
    // Toward zero from both sides.
    let a = toInt32(42.9)      // 42
    let b = toInt32(-2.9)      // -2
    let c = fromFloat(7.6)     // 7
    let d = Int32(toInt64(1000.5))  // 1000
    let e = Int32(unsignedFromDouble(3.99))  // 3

    // At the edges of the range, which is where an inexact bound
    // would show. Compared rather than added: the sum would overflow
    // Int32, which is a trap about this program's arithmetic and not
    // about the conversion.
    let lo = toInt32(-2147483648.0)  // Int32.min
    let hi = toInt32(2147483647.0)   // Int32.max
    let edges: Int32 = (lo < -2147483647 && hi > 2147483646) ? 0 : 100

    return a + b + c + (d - 1000) + e + edges
}
