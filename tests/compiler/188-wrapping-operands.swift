// The same with the operands in variables, so nothing is folded.
func add(_ a: Int32, _ b: Int32) -> Int32 { return a &+ b }
func sub(_ a: Int32, _ b: Int32) -> Int32 { return a &- b }
func mul(_ a: Int32, _ b: Int32) -> Int32 { return a &* b }
func shl(_ a: Int32, _ b: Int32) -> Int32 { return a &<< b }
func shr(_ a: UInt32, _ b: UInt32) -> UInt32 { return a &>> b }
func main() -> Int32 {
    var n: Int32 = 0
    if add(2147483647, 1) == -2147483648 { n += 1 }
    if sub(-2147483648, 1) == 2147483647 { n += 2 }
    if mul(2147483647, 2) == -2 { n += 4 }
    if shl(1, 33) == 2 { n += 8 }
    if shr(0xF0000000, 36) == 0x0F000000 { n += 16 }
    if add(2, 3) == 5 { n += 32 }
    return n
}
