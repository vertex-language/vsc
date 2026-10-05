// &, |, ^ and ~ on signed and unsigned values.
let a = 0b1100
let b = 0b1010
print(a & b, a | b, a ^ b, ~a)
let x: UInt8 = 0xF0
print(~x, x & 0x3C, x | 0x0F, x ^ 0xFF)
