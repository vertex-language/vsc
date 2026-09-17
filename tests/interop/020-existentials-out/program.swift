// Compiled by this compiler, linked against the library above.
import Shapes

struct Dot: Measured {
    func measure() -> Int32 { return 1 }
    func scaled(_ k: Int32) -> Int32 { return k }
}

struct Bar: Measured {
    var length: Int32
    func measure() -> Int32 { return length }
    func scaled(_ k: Int32) -> Int32 { return length * k }
}

// Three words, which is the widest a value may be and still sit in
// the buffer rather than in a box.
struct Wide: Measured {
    var a: Int64
    var b: Int64
    var c: Int64
    func measure() -> Int32 { return Int32(a + b + c) }
    func scaled(_ k: Int32) -> Int32 { return Int32(a) * k }
}

func main() -> Int32 {
    if measureIt(Dot()) != 1 { return 91 }
    if measureIt(Bar(length: 7)) != 7 { return 92 }
    if measureIt(Wide(a: 1, b: 2, c: 3)) != 6 { return 93 }

    // A requirement with an argument, which is the row after the
    // first one.
    if scaleIt(Dot(), 5) != 5 { return 94 }
    if scaleIt(Bar(length: 3), 4) != 12 { return 95 }

    // Copied and destroyed by the callee, both through the value
    // witness table the metadata carries.
    if twice(Bar(length: 8)) != 16 { return 96 }
    if twice(Wide(a: 10, b: 0, c: 0)) != 20 { return 97 }

    // Two at once.
    if sum(Dot(), Bar(length: 5)) != 6 { return 98 }

    return sum(Bar(length: 20), Bar(length: 20)) + measureIt(Bar(length: 2))
}
