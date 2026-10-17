// A mutable pointer passed where an immutable one is wanted converts.
let p = UnsafeMutablePointer<UInt8>.allocate(capacity: 4)
for i in 0..<4 { p[i] = UInt8(i * 10) }
let b = UnsafeBufferPointer(start: p, count: 4)
print(Array(b))
func sum(_ q: UnsafePointer<UInt8>, _ n: Int) -> Int { (0..<n).reduce(0) { $0 + Int(q[$1]) } }
print(sum(p, 4))
p.deallocate()
