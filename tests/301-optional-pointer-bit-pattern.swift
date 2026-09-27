// The bit pattern of an optional pointer: zero for nil.
let none: UnsafeRawPointer? = nil
print(Int(bitPattern: none), UInt(bitPattern: none))
let p = UnsafeRawPointer(bitPattern: 0x1000)
print(Int(bitPattern: p), UInt(bitPattern: p))
let m: UnsafeMutablePointer<Int>? = UnsafeMutablePointer(bitPattern: 0x2000)
print(Int(bitPattern: m))
