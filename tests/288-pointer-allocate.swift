// Memory a program allocates and frees itself: allocate(capacity:),
// initialize(repeating:count:), subscripts, deinitialize and deallocate.
let p = UnsafeMutablePointer<Int>.allocate(capacity: 4)
p.initialize(repeating: 7, count: 4)
for i in 0..<4 { p[i] += i }
print(p[0], p[1], p[2], p[3])
p.deinitialize(count: 4)
p.deallocate()

let raw = UnsafeMutableRawPointer.allocate(byteCount: 16, alignment: 8)
raw.storeBytes(of: 0x11223344, as: UInt32.self)
raw.storeBytes(of: 0x55, toByteOffset: 4, as: UInt8.self)
print(raw.load(as: UInt32.self), raw.load(fromByteOffset: 4, as: UInt8.self))
let ints = raw.assumingMemoryBound(to: UInt32.self)
print(ints[0])
raw.deallocate()
