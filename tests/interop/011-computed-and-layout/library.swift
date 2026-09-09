// What a type's bytes are, and what only looks like one of them.
//
// A computed property has no storage: `v.magnitude` is a call to a
// getter that takes the receiver, and swiftc names it
// `$s...9magnitudes5Int32Vvg` -- v for a variable, g for its getter.
// A static property is the type's storage rather than an instance's.
//
// Counting either as a field is worse than reading the wrong bytes,
// because it changes what the type's bytes are. Vec below would be
// sixteen bytes rather than eight: it would cross a call in two
// registers where swiftc passes one, and everything after it would
// be at the wrong offset. That is what this case holds -- the
// arithmetic is trivial and the layout is the point.
public struct Vec {
    public var x: Int32
    public var y: Int32
    public init(x: Int32, y: Int32) { self.x = x; self.y = y }

    public var magnitude: Int32 { return x * x + y * y }

    // Members of the type rather than of an instance: a static
    // method has no receiver to pass, and a static stored property
    // has storage of its own reached through an accessor rather than
    // at an offset in anything.
    public static let unit: Int32 = 1
    public static func zero() -> Vec { return Vec(x: 0, y: 0) }
    public static func of(_ n: Int32) -> Vec { return Vec(x: n, y: n) }
}

// A stored property after a computed one, which is where a wrong
// layout shows up as a wrong field.
public struct Mixed {
    public var a: Int32
    public var half: Int32 { return a / 2 }
    public var b: Int32
    public init(a: Int32, b: Int32) { self.a = a; self.b = b }
}

public func plus(_ a: Vec, _ b: Vec) -> Vec { return Vec(x: a.x + b.x, y: a.y + b.y) }
public func lengthOf(_ v: Vec) -> Int32 { return v.magnitude }
public func spread(_ m: Mixed) -> Int32 { return m.a + m.b }
