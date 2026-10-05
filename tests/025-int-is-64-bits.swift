// Int and UInt are word sized: 64 bits here, and the same as Int64.
print(Int.bitWidth, UInt.bitWidth, Int8.bitWidth, Int16.bitWidth, Int32.bitWidth)
print(Int.max == Int(Int64.max), Int.min == Int(Int64.min))
print(MemoryLayout<Int>.size, MemoryLayout<Int8>.size, MemoryLayout<Int16>.size, MemoryLayout<Int32>.size)
