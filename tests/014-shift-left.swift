// << in range, and the smart shift past the width, which gives zero.
let one = 1
print(one << 0, one << 1, one << 10, one << 62)
let u: UInt8 = 0b1011_0001
print(u << 1, u << 4, u << 8, u << 100)
print(one << -1)
