// Counting and rearranging bits.
let x: UInt32 = 0b0000_0000_1011_0000_0000_0000_0000_0001
print(x.nonzeroBitCount, x.leadingZeroBitCount, x.trailingZeroBitCount)
print(x.byteSwapped, UInt16(0x1234).byteSwapped, (8 as Int).trailingZeroBitCount)
print(Int(0).leadingZeroBitCount, (-1 as Int8).nonzeroBitCount)
