// Writing through an optional chain into a struct: `h.inner?.n = 5` stores
// into the payload where the optional is some and does nothing where it is
// nil, as does a compound assignment, a mutating call and a bare `x?`.
// A class's `var` of an optional type starts as nil. An optional of a
// struct that itself holds an optional struct carries its references.

struct Leaf { var n = 0; var name = "leaf"; mutating func rename(_ s: String) { name = s } }
struct Mid { var leaf: Leaf?; var count = 0 }
struct Top { var mid: Mid? }
final class Box { var top: Top? }
final class Holder { var mid: Mid? = nil; var mids: [Mid?] = [] }
var global: Leaf? = Leaf()

func describe(_ m: Mid?) -> String {
    if let m = m, let l = m.leaf { return "\(l.name)/\(m.count)" }
    return "-"
}

func main() -> Int32 {
    var t = Top(mid: Mid(leaf: Leaf()))
    t.mid?.leaf?.n = 3
    t.mid?.leaf?.name = "x"
    t.mid?.leaf?.rename("y")
    t.mid?.count += 1
    print(t.mid!.leaf!.n, t.mid!.leaf!.name, t.mid!.count)
    t.mid?.leaf = nil
    t.mid?.leaf?.n = 9
    print(t.mid!.leaf == nil)

    let b = Box()
    b.top?.mid?.count = 5
    print(b.top == nil)
    b.top = Top(mid: Mid(leaf: nil))
    b.top?.mid?.count = 5
    b.top?.mid?.leaf?.n = 1
    print(b.top!.mid!.count, b.top!.mid!.leaf == nil)

    global?.n = 42
    global?.name += "!"
    print(global!.n, global!.name)

    var arr: [Leaf?] = [Leaf(), nil]
    arr[0]?.n = 7
    arr[1]?.n = 8
    print(arr[0]!.n, arr[1] == nil)

    var s: String? = "a"
    s?.append("b")
    s? += "c"
    print(s!)
    var n: Int? = 1
    n? += 2
    print(n!)
    var xs: [Int]? = [1]
    xs?.append(2)
    xs?[0] = 10
    print(xs!)

    var mids: [Mid?] = [Mid(leaf: Leaf(n: 1, name: "a"), count: 1), nil, Mid(leaf: nil, count: 2)]
    mids.append(Mid(leaf: Leaf(n: 2, name: "b"), count: 3))
    for m in mids { print(describe(m)) }
    let ys = mids
    mids[0] = nil
    print(describe(ys[0]), describe(mids[0]))
    let h = Holder()
    h.mid = ys[3]
    h.mids = ys
    h.mids[3]?.leaf?.name += "!"
    h.mid?.count = 9
    print(describe(h.mid), describe(h.mids[3]))
    var pair: (Mid?, String) = (nil, "t")
    pair.0 = Mid(leaf: Leaf(n: 5, name: "tt"), count: 5)
    print(describe(pair.0), pair.1)
    return Int32(t.mid!.count)
}
