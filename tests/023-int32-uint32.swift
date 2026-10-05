// Int32 and UInt32: arithmetic in range, and their bounds.
let a: Int32 = -2_000_000_000
let b: Int32 = 7
print(a / b, a % b, a + 147_483_647, Int32.min, Int32.max)
let c: UInt32 = 4_294_967_295
print(c / 2, c % 1000, UInt32.max)
