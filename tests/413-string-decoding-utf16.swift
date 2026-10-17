// String(decoding:as: UTF16.self), as a GPT partition name is stored.
let units: [UInt16] = [0x0062, 0x006f, 0x006f, 0x0074, 0x00e9, 0xd83d, 0xde00]
let s = String(decoding: units, as: UTF16.self)
print(s, s.count)
let name = String(decoding: [0x45, 0x46, 0x49] as [UInt16], as: UTF16.self)
print(name)
print(String(decoding: [0xd800] as [UInt16], as: UTF16.self).unicodeScalars.first!.value)
