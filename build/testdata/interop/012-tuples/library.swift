// A tuple is not a struct, and the difference is only visible at a
// call.
//
// swiftc's own code for `sumTuple(_ t: (Int32, Int32))` reads w0 and
// w1 -- the elements, one register each. Its code for `plus(_ a: Vec,
// _ b: Vec)`, where Vec has the same two fields, reads x0 and x1,
// each holding a whole Vec packed into a word. A tuple parameter is
// the parameter list and Swift flattens it into one; a struct
// parameter is a value.
//
// A tuple *result* is packed like a struct's, which is the other half
// of the same question and the reason both are here.
public struct Vec {
    public var x: Int32
    public var y: Int32
    public init(x: Int32, y: Int32) { self.x = x; self.y = y }
}

public func pairOf(_ n: Int32) -> (Int32, Int32) { return (n, n + 1) }
public func sumTuple(_ t: (Int32, Int32)) -> Int32 { return t.0 + t.1 }
public func labelled() -> (lo: Int32, hi: Int32) { return (lo: 1, hi: 41) }

// A tuple beside an ordinary argument, which is where a flattening
// that went the wrong way shows up as a shifted argument list.
public func after(_ t: (Int32, Int32), _ n: Int32) -> Int32 { return n }
public func before(_ n: Int32, _ t: (Int32, Int32)) -> Int32 { return t.1 }

// And a struct with the same shape, so the two conventions sit beside
// each other.
public func vecSum(_ v: Vec) -> Int32 { return v.x + v.y }
