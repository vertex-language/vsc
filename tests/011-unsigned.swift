// UInt arithmetic: division and remainder are unsigned, and so is the compare.
let a: UInt = 18_446_744_073_709_551_615
let b: UInt = 10
print(a / b, a % b, a > b, a - 5)
let c: UInt32 = 4_000_000_000
print(c / 3, c % 7, c + 1)
