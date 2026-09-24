// The other direction: @_silgen_name names a symbol somebody else
// defines, and the C file below defines these. Nothing is imported --
// the declaration is the whole of what this compiler is told.
@_silgen_name("c_double")
func cDouble(_ x: Int32) -> Int32

@_silgen_name("c_max")
func cMax(_ a: Int32, _ b: Int32) -> Int32

@_cdecl("vs_via_c")
func viaC(_ x: Int32) -> Int32 { return cDouble(x) + 1 }

@_cdecl("vs_biggest")
func biggest(_ a: Int32, _ b: Int32, _ c: Int32) -> Int32 {
    return cMax(cMax(a, b), c)
}

// A round trip: C calls this, it calls C, and the answer comes back.
@_cdecl("vs_round_trip")
func roundTrip(_ x: Int32) -> Int32 { return cDouble(cDouble(x)) }
