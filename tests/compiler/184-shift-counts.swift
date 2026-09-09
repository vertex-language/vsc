// The same, with the counts in variables so nothing is folded, and
// unsigned counts alongside signed ones.
func sl(_ a: Int32, _ b: Int32) -> Int32 { return a << b }
func sr(_ a: Int32, _ b: Int32) -> Int32 { return a >> b }
func ul(_ a: UInt32, _ b: UInt32) -> UInt32 { return a << b }
func ur(_ a: UInt32, _ b: UInt32) -> UInt32 { return a >> b }
func main() -> Int32 {
    var n: Int32 = 0
    if sl(1, 40) == 0 { n += 1 }
    if sl(1, -2) == 0 { n += 2 }
    if sr(-256, 40) == -1 { n += 4 }
    if sr(-256, -2) == -1024 { n += 8 }
    if sl(3, 4) == 48 { n += 16 }
    if sr(-16, 2) == -4 { n += 32 }
    if ul(1, 40) == 0 { n += 64 }
    if ur(0xF0000000, 40) == 0 { n += 128 }
    if ur(0xF0000000, 28) == 15 { n += 256 }
    return n % 128
}
