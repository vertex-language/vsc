// UnsafeMutablePointer.update(from:count:) copies elements in place.
let src: [Int] = [1, 2, 3, 4]
let p = UnsafeMutablePointer<Int>.allocate(capacity: 4)
p.initialize(repeating: 0, count: 4)
src.withUnsafeBufferPointer { b in p.update(from: b.baseAddress!, count: 3) }
print((0..<4).map { p[$0] })
p.deinitialize(count: 4)
p.deallocate()
