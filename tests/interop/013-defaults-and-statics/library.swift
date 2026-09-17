// Two ways a call is not what it looks like.
//
// A parameter with a default may be left out, and the caller supplies
// the value -- Swift evaluates it at the call, which is why a
// client's object file references the function and nothing else. A
// call that passed only what was written would leave the callee
// reading a register nobody wrote.
//
// And a member of the type is not a member of an instance: a static
// method has no receiver to pass, and a static stored property has
// storage of its own reached through an accessor rather than at an
// offset in anything.
public struct Scale {
    public var factor: Int32
    public init(factor: Int32) { self.factor = factor }

    public func applied(to n: Int32, times k: Int32 = 1) -> Int32 {
        return n * factor * k
    }

    public static let identity: Int32 = 1
    public static func of(_ n: Int32) -> Scale { return Scale(factor: n) }
}

public func shifted(_ n: Int32, by k: Int32 = 2) -> Int32 { return n + k }

// A parameter the callee writes through: what crosses the call is the
// caller's storage rather than a copy of what is in it.
public func bump(_ n: inout Int32, by k: Int32 = 1) { n = n + k }
public func exchange(_ a: inout Int32, _ b: inout Int32) { let t = a; a = b; b = t }
public func between(_ n: Int32, low: Int32 = 0, high: Int32 = 100) -> Int32 {
    if n < low { return low }
    if n > high { return high }
    return n
}
