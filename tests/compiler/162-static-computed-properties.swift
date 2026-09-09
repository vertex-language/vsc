// A computed property of the type rather than of an instance. It is a
// getter with nothing behind it -- no storage, no one-time
// initializer -- so it is a call, and the symbol is the instance
// getter's name with Z after it, which is what swiftc writes.
//
// It used to be counted among the type's stored statics, which are
// the ones that do need storage, and was refused for wanting a global
// it has no use for.
struct Vec {
    var x: Int32
    var y: Int32

    static var zero: Vec { return Vec(x: 0, y: 0) }
    static var unit: Vec { return Vec(x: 1, y: 1) }

    // One that reads another, so the getter is a call from a getter.
    // Written bare: a type's statics are in scope unqualified inside
    // its own members, the way its properties are through self.
    static var two: Vec { return Vec(x: unit.x * 2, y: unit.y * 2) }

    var sum: Int32 { return x + y }
}

class Limits {
    static var high: Int32 { return 100 }
    static var low: Int32 { return 1 }
}

func main() -> Int32 {
    let a = Vec.zero.sum          // 0
    let b = Vec.unit.sum          // 2
    let c = Vec.two.sum           // 4
    let d = Limits.high - Limits.low  // 99
    return a + b + c + d
}
