// Pointers across the boundary, which is what a C signature is made
// of: an address in, a read or a write through it, and nothing owned
// on either side.
@_cdecl("vs_read")
func read(_ p: UnsafePointer<Int32>) -> Int32 { return p.pointee }

@_cdecl("vs_write")
func write(_ p: UnsafeMutablePointer<Int32>, _ v: Int32) { p.pointee = v }

@_cdecl("vs_bump")
func bump(_ p: UnsafeMutablePointer<Int32>) { p.pointee += 1 }

@_cdecl("vs_swap")
func swapThrough(_ a: UnsafeMutablePointer<Int32>, _ b: UnsafeMutablePointer<Int32>) {
    let t = a.pointee
    a.pointee = b.pointee
    b.pointee = t
}

// Two out-parameters, which is the shape a C function takes when it
// has more than one answer.
@_cdecl("vs_divmod")
func divmod(_ a: Int32, _ b: Int32,
            _ q: UnsafeMutablePointer<Int32>, _ r: UnsafeMutablePointer<Int32>) {
    q.pointee = a / b
    r.pointee = a % b
}

// The other direction: storage C owns, reached from here.
@_silgen_name("c_alloc_int")
func cAllocInt(_ v: Int32) -> UnsafeMutablePointer<Int32>

@_silgen_name("c_free_int")
func cFreeInt(_ p: UnsafeMutablePointer<Int32>)

@_cdecl("vs_round_trip")
func roundTrip(_ v: Int32) -> Int32 {
    let p = cAllocInt(v)
    p.pointee += 10
    let out = p.pointee
    cFreeInt(p)
    return out
}

// Two addresses are equal when they are the same place.
@_cdecl("vs_same")
func same(_ a: UnsafePointer<Int32>, _ b: UnsafePointer<Int32>) -> Bool {
    return a == b
}
