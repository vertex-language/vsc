// Pointers made writable with init(mutating:): a typed one written
// through, and a raw one of the same address.
var xs: [Int32] = [1, 2, 3]
xs.withUnsafeBufferPointer { buf in
    let p = UnsafeMutablePointer<Int32>(mutating: buf.baseAddress!)
    p[1] = 20
    p[2] = p[0] + p[1]
    let raw = UnsafeMutableRawPointer(mutating: buf.baseAddress!)
    print(raw == UnsafeMutableRawPointer(p))
}
print(xs)
