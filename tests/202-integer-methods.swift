// The integer methods beside the operators.
let n = -17
print(n.quotientAndRemainder(dividingBy: 5), n.signum(), n.magnitude, abs(n))
print(12.isMultiple(of: 4), 12.isMultiple(of: 5), 0.isMultiple(of: 0))
print(UInt8(0b1001_0110).leadingZeroBitCount, Int.max.bitWidth, (255 as UInt8).words.first!)
print(min(3, 9, -2), max(3, 9, -2), 7.distance(to: 2), 7.advanced(by: -10))
