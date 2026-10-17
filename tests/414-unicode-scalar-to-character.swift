// Unicode.Scalar from a byte and a UInt32, made into a Character.
var s = ""
for byte: UInt8 in [0x68, 0x69, 0x21] { s.append(Character(Unicode.Scalar(byte))) }
print(s)
if let e = Unicode.Scalar(0x20ac as UInt32) { print(Character(e), e.value) }
print(Unicode.Scalar(0xd800 as UInt32) == nil)
