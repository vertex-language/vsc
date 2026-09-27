// A range of a narrow integer is a collection, as any range of a
// Strideable with a signed-integer stride is.
let a = Array(UInt8(1)...3)
print(a)
let b = (Int16(-2)..<2).map { $0 * 10 }
print(b)
var bytes: [UInt8] = []
bytes.append(contentsOf: 65...67)
print(bytes)
let r: ClosedRange<UInt8> = 10...12
print(r.count, r.contains(11), r.reduce(0) { $0 + Int($1) })
