// truncatingIfNeeded keeps the low bits; bitPattern reinterprets them.
let big = 0x1234_5678
print(UInt8(truncatingIfNeeded: big), Int8(truncatingIfNeeded: 0xFF), UInt16(truncatingIfNeeded: -1))
print(UInt8(bitPattern: -1), Int8(bitPattern: 0x80), Int(bitPattern: UInt.max))
print(Int32(clamping: 5_000_000_000), UInt8(clamping: -3), Int8(clamping: 90))
