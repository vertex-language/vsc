// A subclass's storage begins where the base's ends.
class Base {
    var a: Int32 = 1
    var b: Int32 = 2
}

class Middle: Base {
    var c: Int32 = 3
}

class Leaf: Middle {
    var d: Int32 = 4
}

func main() -> Int32 {
    let l = Leaf()
    l.a = 10
    l.b = 20
    l.c = 30
    l.d = 40
    // Each write has to land somewhere of its own.
    if l.a != 10 { return 91 }
    if l.b != 20 { return 92 }
    if l.c != 30 { return 93 }
    if l.d != 40 { return 94 }

    // And the base still sees its own through a base-typed name.
    let b: Base = l
    if b.a != 10 { return 95 }
    if b.b != 20 { return 96 }
    return 42
}
