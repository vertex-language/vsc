// Int64 and UInt64: arithmetic in range, and their bounds.
let a: Int64 = -9_000_000_000_000_000_000
let b: Int64 = 3
print(a / b, a % b, Int64.min, Int64.max)
let c: UInt64 = 18_000_000_000_000_000_000
print(c / 1_000_000_007, c % 1_000_000_007, UInt64.max)
