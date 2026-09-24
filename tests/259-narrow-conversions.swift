// Conversions through a narrow integer nested in one expression: the truncation is kept.
let i = 250
print(Int32(Int8(truncatingIfNeeded: i)), Int32(Int8(truncatingIfNeeded: i + 1)))
print(Int(UInt8(truncatingIfNeeded: 300)), Int(Int16(truncatingIfNeeded: 40000)))
let a: Int8 = -1
let b = UInt8(bitPattern: a)
print(b, b > 200, UInt32(b), Int32(Int8(bitPattern: b)))
