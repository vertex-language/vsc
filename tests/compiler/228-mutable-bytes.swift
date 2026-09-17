// An array lends its bytes to a closure to write through; the closure's
// result, known from the call's context, is the call's. A raw pointer
// into the bytes moves by an Int.
@_silgen_name("memset")
func c_memset(_ p: UnsafeMutableRawPointer?, _ c: Int32, _ n: Int) -> UnsafeMutableRawPointer?

func fill(_ buffer: inout [UInt8], from offset: Int) -> Int {
    return buffer.withUnsafeMutableBytes { raw in
        _ = c_memset(raw.baseAddress! + offset, 7, raw.count - offset)
        return raw.count
    }
}

func main() -> Int32 {
    var bytes = [UInt8](repeating: 1, count: 6)
    let before = bytes
    let n = fill(&bytes, from: 2)
    var total = n
    for b in bytes {
        total += Int(b)
    }
    for b in before {
        total += Int(b) * 100
    }
    var wide = [Int32](repeating: 0, count: 3)
    let m = wide.withUnsafeMutableBytes { raw in raw.count }
    total += m
    return Int32(total % 251)
}
