// >> is arithmetic on signed types and logical on unsigned ones.
let n = -64
print(n >> 1, n >> 3, n >> 100)
let p = 1 << 40
print(p >> 20, p >> 64)
let u: UInt16 = 0x8000
print(u >> 1, u >> 15, u >> 16)
let s: Int16 = -0x8000
print(s >> 1, s >> 15)
