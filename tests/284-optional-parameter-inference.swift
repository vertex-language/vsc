// A type parameter is inferred through an optional parameter a plain
// value is passed to -- a generic function's, a memberwise initializer's,
// an initializer an extension declares -- and through a pointer's
// element: UnsafeBufferPointer(start:count:). And removeSubrange.
func first<T>(_ x: T?) -> T { return x! }
print(first(3), first("a"))

struct Box<T> {
    let v: T?
    let n: Int
}
extension Box {
    init(start: T?, count: Int) { self.init(v: start, n: count) }
}
let a = Box(v: 1.5, n: 1)
let b = Box(start: "two", count: 2)
print(a.v!, b.v!, b.n)

let bytes: [UInt8] = [10, 20, 30]
bytes.withUnsafeBufferPointer { p in
    let q = UnsafeBufferPointer(start: p.baseAddress, count: 2)
    print(q.count, q[1])
}

var xs = [1, 2, 3, 4, 5]
xs.removeSubrange(1..<3)
print(xs)
