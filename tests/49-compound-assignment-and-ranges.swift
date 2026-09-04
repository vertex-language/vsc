// §2.4 / §4.2 Range Operators and Compound Assignment

func testRanges() {
    let closed = 1...10
    let halfOpen = 0..<10
    let oneSidedTo = ...100
    let oneSidedUpTo = ..<100
    let oneSidedFrom = 50...

    let array = [10, 20, 30, 40, 50]
    let slice1 = array[1...3]
    let slice2 = array[..<2]
    let slice3 = array[2...]

    _ = (closed, halfOpen, oneSidedTo, oneSidedUpTo, oneSidedFrom, slice1, slice2, slice3)
}

func testCompoundAssignment() {
    var x = 100
    x += 10
    x -= 5
    x *= 2
    x /= 3
    x %= 4

    var bits: UInt8 = 0b0000_1111
    bits &= 0b1010_1010
    bits |= 0b0101_0101
    bits ^= 0b1111_0000
    bits <<= 1
    bits >>= 2

    _ = (x, bits)
}
