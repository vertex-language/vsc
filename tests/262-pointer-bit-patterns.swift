// A pointer's bit pattern and back: Int(bitPattern:) and UInt(bitPattern:)
// of a pointer, and the failable init?(bitPattern:), nil at zero.
var xs: [Int32] = [10, 20, 30, 40]
xs.withUnsafeMutableBufferPointer { buf in
    let p = buf.baseAddress!
    let a = UInt(bitPattern: p)
    let i = Int(bitPattern: p + 2)
    print(i - Int(bitPattern: a) == 8, a % 4 == 0)
    let q = UnsafeMutablePointer<Int32>(bitPattern: a + 4)!
    print(q.pointee)
    q.pointee = 99
    let r = UnsafeMutablePointer<Int32>(bitPattern: i)
    print(r == nil, r!.pointee)
    print(UnsafeMutablePointer<Int32>(bitPattern: 0) == nil, UnsafeMutableRawPointer(bitPattern: UInt(0)) == nil)
    let raw = UnsafeRawPointer(bitPattern: a)
    print(raw != nil)
}
print(xs)
