// A generic function over pointers, its T inferred from the pointer given:
// an UnsafePointer<T> of Int32s and of UInt8s, an UnsafeMutablePointer<T>.
func sum<T: BinaryInteger>(_ p: UnsafePointer<T>, _ n: Int) -> Int {
    var s = 0
    for i in 0..<n { s += Int(p[i]) }
    return s
}

func first<T>(_ p: UnsafeMutablePointer<T>) -> T {
    return p.pointee
}

let xs: [Int32] = [1, 2, 3]
xs.withUnsafeBufferPointer { print(sum($0.baseAddress!, $0.count)) }
let bytes: [UInt8] = [250, 5]
bytes.withUnsafeBufferPointer { print(sum($0.baseAddress!, 2)) }
var ys: [Double] = [2.5, 1]
ys.withUnsafeMutableBufferPointer { print(first($0.baseAddress!)) }
