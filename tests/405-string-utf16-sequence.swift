// String.utf16 walked as a Sequence: for-in, map, count, Array.
let s = "aé€😀"
var units: [UInt16] = []
for u in s.utf16 { units.append(u) }
print(units)
print(s.utf16.count, s.utf16.map { $0 > 0x7f }.filter { $0 }.count)
print(Array("hi".utf16))
