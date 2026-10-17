// An array of bytes made from an UnsafeRawBufferPointer.
let p = UnsafeMutableRawPointer.allocate(byteCount: 6, alignment: 1)
for i in 0..<6 { p.storeBytes(of: UInt8(i * 3), toByteOffset: i, as: UInt8.self) }
let bytes = [UInt8](UnsafeRawBufferPointer(start: UnsafeRawPointer(p), count: 6))
print(bytes)
var n = 0
for b in UnsafeRawBufferPointer(start: UnsafeRawPointer(p), count: 6) { n += Int(b) }
print(n)
p.deallocate()
