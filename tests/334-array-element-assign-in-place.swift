// Assigning array elements one at a time does not copy the array each time:
// a byte loop over 2,000,000 elements finishes well inside the time limit.
var bytes = [UInt8](repeating: 0, count: 2_000_000)
for i in 0..<bytes.count {
    bytes[i] = UInt8(truncatingIfNeeded: i &* 31)
}
for i in 1..<bytes.count {
    bytes[i] ^= bytes[i - 1]
}
var sum = 0
for b in bytes { sum += Int(b) }
print(sum)
