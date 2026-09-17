// Unsafe pointers: `&x` hands over where a variable lives, `pointee`
// reads and writes through an address, and one pointer read as
// another changes nothing the machine does.
func read(_ p: UnsafePointer<Int32>) -> Int32 { return p.pointee }
func write(_ p: UnsafeMutablePointer<Int32>, _ v: Int32) { p.pointee = v }
func bump(_ p: UnsafeMutablePointer<Int32>) { p.pointee += 1 }

func divmod(_ a: Int32, _ b: Int32,
            _ q: UnsafeMutablePointer<Int32>, _ r: UnsafeMutablePointer<Int32>) {
    q.pointee = a / b
    r.pointee = a % b
}

func orElse(_ p: UnsafePointer<Int32>?, _ fallback: Int32) -> Int32 {
    guard let q = p else { return fallback }
    return q.pointee
}

// One address, read as an opaque pointer and back again.
func writeThrough(_ p: UnsafeMutablePointer<Int32>) {
    let q = UnsafeMutablePointer<Int32>(OpaquePointer(p))
    q.pointee = 9
}

func main() -> Int32 {
    var n: Int32 = 0
    var x: Int32 = 7

    if read(&x) == 7 { n += 1 }
    write(&x, 42)
    if x == 42 { n += 2 }
    bump(&x)
    if x == 43 { n += 4 }

    var q: Int32 = 0
    var r: Int32 = 0
    divmod(17, 5, &q, &r)
    if q == 3 && r == 2 { n += 8 }

    if orElse(&x, -1) == 43 { n += 16 }
    if orElse(nil, -1) == -1 { n += 32 }

    // The same address, spelled three ways. It goes through a named
    // pointer first because `OpaquePointer(&y)` is ambiguous in
    // Swift -- `&y` is either pointer, and the initializer takes
    // both.
    var y: Int32 = 5
    writeThrough(&y)
    if y == 9 { n += 64 }

    return n % 128
}
