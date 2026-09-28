// A narrow integer made by a conversion compares, divides and shifts by its
// own width: UInt16(0xDE00) is 0xDE00 whatever its register holds above.
// (Inside a function, where the values stay in registers.)
func isLow(_ u: UInt16) -> Bool { return u >= 0xDC00 && u <= 0xDFFF }
func run() {
    let a: UInt32 = 0xDE00
    let low = UInt16(a)
    print(low, low == 0xDE00, low <= 0xDFFF, low > 0x8000, isLow(low))
    let t = UInt16(truncatingIfNeeded: UInt32(0x1_DE00))
    print(t, t == 0xDE00, t / 2, t >> 4, t % 7)
    let b: UInt32 = 200
    let byte = UInt8(b)
    print(byte, byte <= 0xC8, byte < 0xFF, byte / 3, byte >> 1)
    let s: Int32 = -2
    let small = Int8(s)
    print(small, small < 0, small >= -2, small / 2, small >> 1)
    let wide: Int = 60000
    let u = UInt16(wide)
    print(u <= 0xEA60, u == 60000, u &+ 10000, u.multipliedReportingOverflow(by: 2).overflow)
    let max = UInt16(truncatingIfNeeded: UInt32(0xFFFF))
    print(max.addingReportingOverflow(1).overflow, max &+ 1)
}
run()
