// Shifting an unsigned integer by a signed Int amount.
let u: UInt32 = 0xF000_000F
let n = 4
let k: Int = 36
print(u >> n, u << n, u >> k, UInt8(0x81) >> n, UInt64(1) << Int(40))
