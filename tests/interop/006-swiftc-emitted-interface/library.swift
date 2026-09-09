// Built by swiftc, called by this compiler -- and unlike the other
// cases, the interface beside it is not hand-written. It is what
// swiftc itself emitted for this file, copied in unedited.
//
// That is the whole point of the case. A .swiftinterface is valid
// Swift with the bodies taken out, so a compiler that reads Swift can
// read one; this checks that the claim holds for a real one rather
// than for a file written to be easy. What comes with it is swiftc's
// own spelling: every name qualified by its module, including the
// module's own types named through itself.
public struct Point {
    public var x: Int32
    public var y: Int32
    public init(x: Int32, y: Int32) { self.x = x; self.y = y }
    public func sum() -> Int32 { return x + y }
}

public func addAll(_ a: Int32, _ b: Int32) -> Int32 { return a + b }
public func pointSum(_ p: Point) -> Int32 { return p.sum() }
public func scaled(_ p: Point, by n: Int32) -> Point {
    return Point(x: p.x * n, y: p.y * n)
}
