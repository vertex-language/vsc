// Compiled by this compiler, linked against the library above,
// through the interface swiftc emitted for it.
import Geometry

func main() -> Int32 {
    if addAll(40, 2) != 42 { return 91 }

    // A struct declared in another module, made here and read here:
    // the interface listed its stored properties in order, which is
    // its layout, so a field is an offset rather than a call.
    let p = Point(x: 40, y: 2)
    if p.x != 40 { return 92 }
    if p.y != 2 { return 93 }
    if p.sum() != 42 { return 94 }

    // The same value passed back, into a function whose parameter the
    // interface named through the module it is in.
    if pointSum(p) != 42 { return 95 }

    let bigger = scaled(p, by: 3)
    if bigger.x != 120 { return 96 }
    if bigger.sum() != 126 { return 97 }

    return pointSum(Point(x: 20, y: 22))
}
