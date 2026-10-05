// Compiled by this compiler, linked against the library above.
import Geometry2

func main() -> Int32 {
    // Two of them passed and one returned, all in one register each.
    // With the computed property counted as storage this returned 20:
    // the second argument went where swiftc was not looking.
    let sum = plus(Vec(x: 20, y: 0), Vec(x: 22, y: 0))
    if sum.x != 42 { return 91 }
    if sum.y != 0 { return 92 }

    // The getter, called from here.
    let v = Vec(x: 6, y: 1)
    if v.magnitude != 37 { return 93 }
    if lengthOf(v) != 37 { return 94 }

    // A stored property after a computed one.
    let m = Mixed(a: 20, b: 22)
    if m.a != 20 { return 95 }
    if m.b != 22 { return 96 }
    if spread(m) != 42 { return 97 }

    // The type's own members.
    if Vec.zero().x != 0 { return 98 }
    if Vec.of(21).y != 21 { return 99 }
    if Vec.unit != 1 { return 100 }

    return sum.x
}
