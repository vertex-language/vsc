// Unsigned integers of every width converted straight to Double.
let a: UInt32 = 4_000_000_000
let b: UInt64 = 1 << 63
let c: UInt8 = 255
let d: UInt16 = 65_535
print(Double(a), Double(b), Double(c), Double(d), Float(a))
