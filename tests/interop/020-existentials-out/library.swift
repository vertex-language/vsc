// An existential this compiler fills in, handed to a Swift function.
//
// The other direction from case 016. What goes across is five words:
// three of buffer, then the metadata for whatever is inside, then the
// table of that type's implementations. The buffer and the table were
// always right; what was missing was the metadata, because a type
// declared in the program being compiled had none.
//
// It has one now, and everything the callee does without knowing what
// the value is goes through it: the value is projected through the
// value witness table the metadata carries, copied through it, and
// destroyed through it. The witness call itself reads the table -- one
// row past the conformance descriptor, self as an address in x20, the
// metadata and the table after the arguments the source wrote.
public protocol Measured {
    func measure() -> Int32
    func scaled(_ k: Int32) -> Int32
}

public func measureIt(_ m: any Measured) -> Int32 { m.measure() }

public func scaleIt(_ m: any Measured, _ k: Int32) -> Int32 { m.scaled(k) }

// Held rather than used at once, which makes the callee copy it and
// then destroy it -- both through the value witness table.
public func twice(_ m: any Measured) -> Int32 {
    let held = m
    return held.measure() &+ m.measure()
}

// Two of them, so a caller cannot pass by handing over the same five
// words twice.
public func sum(_ a: any Measured, _ b: any Measured) -> Int32 {
    a.measure() &+ b.measure()
}
