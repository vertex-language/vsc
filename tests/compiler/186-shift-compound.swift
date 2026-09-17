// The compound forms go through the same lowering.
func main() -> Int32 {
    var a: Int32 = 1
    a <<= 40
    var b: Int32 = -256
    b >>= 40
    var c: Int32 = 3
    c <<= 4
    var d: UInt32 = 0xF0000000
    d >>= 40
    return a + b + c + Int32(d) + 50
}
