// Optionals across the boundary.
//
// swiftc's own code for `takeOpt(7)` writes the payload at offset
// zero, a tag byte after it, and loads the eight bytes into x0; the
// callee reads that byte back and compares it with one. So `Int32?`
// is `{ Int32, UInt8 }` -- zero for the case that carries something,
// one for the case that does not -- and it is passed the way that
// struct is passed.
//
// Nothing about that is visible in a signature. A caller that passed
// the payload alone would send four bytes and a tag of whatever
// followed, and the callee would decide it had a value about half the
// time.
public struct Pair {
    public var a: Int32
    public var b: Int32
    public init(a: Int32, b: Int32) { self.a = a; self.b = b }
}

public func orElse(_ v: Int32?, _ d: Int32) -> Int32 { return v ?? d }
public func widthOf(_ v: Int64?, _ d: Int32) -> Int32 {
    if let x = v { return Int32(x) }
    return d
}
public func sumOr(_ v: Pair?, _ d: Int32) -> Int32 {
    if let p = v { return p.a + p.b }
    return d
}

// An optional with an ordinary argument after it: the way this goes
// wrong is not a garbled value but a shifted argument list.
public func pick(_ v: Int32?, _ n: Int32) -> Int32 { return v ?? n }

// And one coming back, which the caller has to read: the tag byte
// swiftc wrote is the one this compiler tests.
public func maybe(_ n: Int32) -> Int32? { return n > 0 ? n : nil }
