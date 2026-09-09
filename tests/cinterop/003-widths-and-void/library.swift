// The widths a C entry point may carry, and a function that answers
// nothing. Every one of these crosses in a register, which is the
// limit @_cdecl is held to here.
@_cdecl("vs_narrow")
func narrow(_ a: Int8, _ b: Int16) -> Int32 { return Int32(a) + Int32(b) }

@_cdecl("vs_unsigned")
func unsigned(_ a: UInt32) -> UInt32 { return a &+ 1 }

@_cdecl("vs_wide")
func wide(_ a: Int64) -> Int64 { return a * 2 }

@_cdecl("vs_float")
func scale(_ x: Double) -> Double { return x * 1.5 }

@_cdecl("vs_flag")
func flag(_ x: Int32) -> Bool { return x > 0 }

// A void entry point. What it did is visible because it hands the
// answer back through C rather than returning it -- which is also the
// shape a callback has, and the only way to see a void call happened.
@_silgen_name("c_record")
func record(_ x: Int32)

@_cdecl("vs_keep")
func keep(_ x: Int32) { record(x * 3) }
