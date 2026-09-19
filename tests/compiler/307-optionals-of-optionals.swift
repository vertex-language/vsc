// Optionals of optionals: `T??` made from a T or a T?, `[T?].popLast()`,
// `x??.y`, `a!!`, matched a level at a time; `a ?? b` where b is an
// optional answers an optional, and `a ?? nil` steps down one level;
// optionals of an Equatable enum or struct are compared by the type's
// own `==`. The `!` and `?` of `x!+1` and `p!=q` are x's and p's.

enum Cursor: Equatable { case arrow, iBeam, hand(Int) }
enum Plain { case a, b }
struct P: Equatable { var x: Int; var y: Int }
struct S { var y: Int }
struct Q { var name: String; var n: Int }
final class Node { let name: String; init(_ n: String) { name = n }; func text() -> String { return name + "!" } }

func find(_ n: Node?) -> Node? { return n }

func lookup(_ k: Int) -> String?? {
    if k == 0 { return nil }
    if k == 1 { return .some(nil) }
    return "v\(k)"
}

func flat(_ s: String??) -> String {
    switch s {
    case .some(.some(let v)): return v
    case .some(.none): return "inner"
    case .none: return "outer"
    }
}

func main() -> Int32 {
    var xs: [Int?] = [1, nil, 3]
    let last = xs.popLast()
    print(last == nil, last! == 3)
    let none = xs.popLast()
    print(none == nil, none! == nil)
    let a: Int?? = 5
    let b: Int?? = .some(nil)
    let c: Int?? = nil
    print(a == nil, b == nil, c == nil, a!!, b! == nil)
    let s: S?? = S(y: 2)
    print(s??.y as Any, s!!.y)
    if let inner = s, let v = inner { print(v.y) }
    switch b {
    case .some(.some(let v)): print("v", v)
    case .some(.none): print("inner nil")
    case .none: print("outer nil")
    }
    print(flat(lookup(0)), flat(lookup(1)), flat(lookup(2)))
    let strs: [String??] = [nil, .some(nil), "a"]
    for x in strs { print(flat(x), x == nil) }
    var qs: [Q?] = [Q(name: "p", n: 1), nil]
    let qLast = qs.popLast()
    print(qLast == nil, qLast! == nil)
    let qFirst = qs.popLast()
    print(qFirst! != nil, qFirst!!.name)
    let t: [Int]?? = [1]
    print(t??.count as Any, t!![0])
    let x: Int? = 4
    print(x!+1, x! + 1)
    let p = 1
    let q: Int? = 2
    print(q!==p)

    // `??` with an optional on the right.
    let hit: Node? = nil
    let other: Node? = Node("o")
    let r = hit ?? find(other)
    print(r?.name ?? "-")
    let r2 = hit ?? find(nil)
    print(r2 == nil)
    let val: String? = nil
    let t1 = val ?? other?.text()
    print(t1 ?? "-")
    let node = Node("n")
    let t2 = val ?? node.text()
    let isArea = true
    let t3 = val ?? (isArea ? node.text() : "")
    print(t2, t3)
    let aa: String?? = "q"
    let bb: String? = aa ?? "z"
    print(bb ?? "-", (aa ?? nil) ?? "-")
    let cc: Int?? = .some(nil)
    let dd = cc ?? 7
    print(dd == nil, dd ?? -1)

    // Comparing optionals of an enum and of a struct.
    let c1: Cursor? = .iBeam
    let c2: Cursor? = nil
    print(c1 == Cursor.iBeam, c1 == .arrow, c2 == .iBeam, c1 == c2, c1 != nil, c1 == .hand(1))
    let h1: Cursor? = .hand(3)
    print(h1 == .hand(3), h1 == .hand(4), h1 != .hand(3))
    let pl: Plain? = .a
    print(pl == .a, pl == .b, pl == nil, pl != .a)
    let pp: P? = P(x: 1, y: 2)
    let pn: P? = nil
    print(pp == P(x: 1, y: 2), pp == P(x: 1, y: 3), pp == nil, pp != P(x: 1, y: 2), pn == pp, pp == pp, pn == pn)
    return Int32(a!!)
}
