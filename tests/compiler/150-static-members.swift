// A member of the type rather than of an instance.
//
// `Vec.zero()` is a static method: it has no receiver to pass, and
// what stands in for one is the metatype -- which for a struct is
// thin, meaning no register at all. Its symbol is the method's with Z
// after it, and reaching it through an instance is not Swift, which
// is why a static and an instance member of the same name are two
// members and not one.
struct Vec {
    var x: Int32
    var y: Int32

    static func zero() -> Vec { return Vec(x: 0, y: 0) }
    static func of(_ n: Int32) -> Vec { return Vec(x: n, y: n) }
    static func sum(_ a: Vec, _ b: Vec) -> Int32 { return a.x + b.x }

    // An instance method beside a static one of the same shape.
    func doubled() -> Vec { return Vec(x: x * 2, y: y * 2) }
}

enum Kind {
    case small
    static func biggest() -> Int32 { return 42 }
}

func main() -> Int32 {
    if Vec.zero().x != 0 { return 91 }
    if Vec.of(21).x != 21 { return 92 }
    if Vec.sum(Vec.of(20), Vec.of(22)) != 42 { return 93 }
    if Vec.of(3).doubled().x != 6 { return 94 }
    if Kind.biggest() != 42 { return 95 }
    return Vec.sum(Vec.of(20), Vec.of(22))
}
