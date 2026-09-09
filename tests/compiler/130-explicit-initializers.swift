// A struct that says how it is made does not also get the memberwise
// initializer for free, so the call goes to the one it declared.
//
// self is a var being filled in rather than a value handed over,
// which is what makes `x = v` legal here and not in an ordinary
// method on a value type.
struct Point {
    var x: Int32
    var y: Int32
    init(v: Int32) { x = v; y = v * 2 }
    init(x: Int32, y: Int32) { self.x = x; self.y = y }
    init() { x = 0; y = 0 }
}

struct Wrapped {
    var inner: Point
    var tag: Int32
    init(_ p: Point) { inner = p; tag = 7 }
}

func main() -> Int32 {
    let a = Point(v: 3)
    if a.x != 3 { return 91 }
    if a.y != 6 { return 92 }

    let b = Point(x: 10, y: 20)
    if b.x != 10 { return 93 }
    if b.y != 20 { return 94 }

    let z = Point()
    if z.x != 0 { return 95 }
    if z.y != 0 { return 96 }

    let w = Wrapped(b)
    if w.inner.y != 20 { return 97 }
    if w.tag != 7 { return 98 }

    return a.x + a.y + b.x + b.y + w.tag - 4
}
