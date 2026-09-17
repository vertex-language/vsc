// A String lends a NUL-terminated copy of its bytes to a closure, and its
// utf8 is those bytes as an array. An array variable lends its elements to
// write through, made its own first, so a copy taken before is untouched.
// String(cString:) reads the bytes before the NUL.
@_silgen_name("strlen")
func c_strlen(_ s: UnsafePointer<CChar>) -> Int

@_silgen_name("memset")
func c_memset(_ p: UnsafeMutablePointer<CChar>?, _ c: Int32, _ n: Int) -> UnsafeMutableRawPointer?

func main() -> Int32 {
    var name = "socket"
    name += "!"
    var total = name.withCString { p in c_strlen(p) }
    var sum = 0
    for b in name.utf8 {
        sum += Int(b)
    }
    total += sum % 100
    var buf = [CChar](repeating: 0, count: 8)
    let copy = buf
    let n = buf.withUnsafeMutableBufferPointer { bp -> Int in
        _ = c_memset(bp.baseAddress, 65, 3)
        return bp.count
    }
    total += n + Int(buf[0]) + Int(copy[0])
    let s = String(cString: buf)
    total += s.count * 10
    return Int32(total % 251)
}
