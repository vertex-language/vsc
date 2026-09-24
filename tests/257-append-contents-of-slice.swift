// append(contentsOf:) taking a slice and a set, reserveCapacity, and String(decoding:) of a slice.
let bytes: [UInt8] = [104, 101, 108, 108, 111, 33]
var out: [UInt8] = []
out.reserveCapacity(8)
out.append(contentsOf: bytes[1..<4])
out.append(contentsOf: Set([UInt8(65)]))
print(out)
print(String(decoding: bytes[0..<5], as: UTF8.self))
