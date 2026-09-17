// An existential a Swift library hands back.
//
// Five words: three of buffer, then the metadata for whatever is
// inside, then the witness table for the conformance. swiftc's own
// `aSquare` writes all three through x8, which is sret, and a caller
// that wants to call a requirement reads the last two back out.
//
// Where the value is depends on what it is. A Square is four bytes
// and lives in the buffer; a Frame is forty and does not, so swiftc
// puts it in a heap box and the buffer holds the box's pointer. Which
// of the two is a fact about the dynamic type, and the only place it
// is written down is the value witness table the metadata carries --
// so a caller that assumes the buffer is right half the time.
public protocol Shape {
    func sides() -> Int32
    func scaled(_ k: Int32) -> Int32
    func between(_ low: Int32, _ high: Int32) -> Int32
}

public struct Square: Shape {
    public var side: Int32
    public init(side: Int32) { self.side = side }
    public func sides() -> Int32 { 4 }
    public func scaled(_ k: Int32) -> Int32 { side * k }
    public func between(_ low: Int32, _ high: Int32) -> Int32 { low + side + high }
}

// Forty bytes, which is more than the three words an existential
// keeps inline, so this one is boxed.
public struct Frame: Shape {
    public var a, b, c, d, e: Int64
    public init(a: Int64) { self.a = a; b = 2; c = 3; d = 4; e = 5 }
    public func sides() -> Int32 { 5 }
    public func scaled(_ k: Int32) -> Int32 { Int32(a) * k }
    public func between(_ low: Int32, _ high: Int32) -> Int32 { low + Int32(e) + high }
}

public func aSquare(_ side: Int32) -> any Shape { Square(side: side) }
public func aFrame(_ a: Int64) -> any Shape { Frame(a: a) }
