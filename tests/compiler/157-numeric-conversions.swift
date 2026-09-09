// `Double(n)` and the conversions around it.
//
// Nothing here can fail. Every integer has a value in a
// floating-point type, and so does every narrower float; what a wide
// integer loses is precision rather than range, and Swift's
// initializer rounds rather than trapping. So the conversion is the
// conversion and nothing else, which is also all `swiftc -O` leaves
// behind.
//
// An integer narrower than a word is widened first, because that is
// what swiftc does: `Double(Int32)` is `sextOrBitCast_Int32_Int64`
// and then `sitofp_Int64_FPIEEE64`, and an unsigned source is
// zero-extended and converted with `uitofp`.
func mean(_ total: Double, _ n: Int32) -> Double {
    if n == 0 { return 0 }
    return total / Double(n)
}

func main() -> Int32 {
    // Signed, at three widths.
    if Double(Int32(3)) != 3.0 { return 91 }
    if Double(Int8(-4)) != -4.0 { return 92 }
    if Double(Int64(1000000)) != 1000000.0 { return 93 }

    // Unsigned, where the top half of the range is what tells
    // zero-extension from sign-extension.
    if Double(UInt32(4000000000)) != 4000000000.0 { return 94 }
    if Double(UInt8(200)) != 200.0 { return 95 }

    // Between the two float widths.
    let f: Float = 1.5
    if Double(f) != 1.5 { return 96 }
    if Float(2.25) != 1.5 + 0.75 { return 97 }

    // A whole number written where a Double is wanted is a Double.
    var d: Double = 0
    d += 1
    if d != 1.0 { return 98 }

    if mean(9.0, 3) != 3.0 { return 99 }
    if mean(1.0, 0) != 0.0 { return 100 }

    return Int32(6) * 7
}
