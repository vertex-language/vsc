// &+, &- and &* wrap instead of trapping.
let big = Int.max
print(big &+ 1 == Int.min)
print(Int.min &- 1 == Int.max)
let b: UInt8 = 200
print(b &+ 100, b &* 3, UInt8(5) &- 10)
let i: Int8 = 100
print(i &* 2, i &+ 100)
