// A left shift of a narrow unsigned integer keeps its bits zero-extended,
// so the result compares equal to a literal with its top bit set.

@inline(never) func shl16(_ a: UInt16, _ n: UInt16) -> UInt16 { return a << n }
@inline(never) func shl8(_ a: UInt8) -> UInt8 { return a << 4 }
@inline(never) func shlSigned(_ a: Int16) -> Int16 { return a << 8 }
@inline(never) func masked(_ a: UInt16) -> UInt16 { return a &<< 12 }

let bytes: [UInt8] = [0xEF, 0xBE]
let word = UInt16(bytes[0]) | (UInt16(bytes[1]) << 8)
print(word == 0xBEEF, word > 0x8000, word)
print(shl16(0xBE, 8) == 0xBE00, shl16(0x1BE, 8))
print(shl8(0xAB) == 0xB0, shl8(0xAB) > 0x80)
print(shlSigned(0x7F), shlSigned(0xBE), shlSigned(0xBE) < 0)
print(masked(0xF) == 0xF000, masked(0xF) > 0x7FFF)
