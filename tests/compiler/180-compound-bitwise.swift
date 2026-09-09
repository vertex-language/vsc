// The bitwise compound assignments.
func main() -> Int32 {
    var n: Int32 = 0b1100
    n &= 0b1010
    n |= 0b0001
    n ^= 0b0010
    n <<= 2
    n >>= 1
    return n
}
