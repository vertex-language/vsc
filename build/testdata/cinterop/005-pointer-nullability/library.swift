// A nullable C pointer is an optional here, and it is one word in
// both: null is a representation an address in use does not have, so
// the empty case takes it and there is no tag byte. That is what
// makes `int *` and `UnsafeMutablePointer<Int32>?` the same argument.
@_cdecl("vs_or_else")
func orElse(_ p: UnsafePointer<Int32>?, _ fallback: Int32) -> Int32 {
    guard let q = p else { return fallback }
    return q.pointee
}

@_cdecl("vs_write_if")
func writeIf(_ p: UnsafeMutablePointer<Int32>?, _ v: Int32) -> Bool {
    guard let q = p else { return false }
    q.pointee = v
    return true
}

@_cdecl("vs_is_null")
func isNull(_ p: UnsafeRawPointer?) -> Bool { return p == nil }

// A null handed back to C, and one that is not.
@_silgen_name("c_maybe")
func cMaybe(_ wantNull: Bool) -> UnsafeMutablePointer<Int32>?

@_cdecl("vs_through")
func through(_ wantNull: Bool) -> Int32 {
    guard let p = cMaybe(wantNull) else { return -1 }
    return p.pointee
}

// Reading one pointer as another changes what the compiler knows and
// nothing the machine does.
@_cdecl("vs_via_raw")
func viaRaw(_ p: UnsafeMutablePointer<Int32>) -> Int32 {
    let raw = UnsafeRawPointer(p)
    let opaque = OpaquePointer(p)
    return isNull(raw) || isNull(UnsafeRawPointer(opaque)) ? 0 : p.pointee
}
