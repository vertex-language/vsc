// A case with a where clause matches only when the clause holds, and when
// it does not, the cases after it are tried: over an enum with trivial,
// counted and wide payloads, and over an optional, spelled every way.
// Over and over, so what a failed case bound is let go of exactly once.

final class Tag { var name: String; init(_ n: String) { name = n } }
enum Shape {
    case circle(Int)
    case rect(Int, Int)
    case named(Tag, String)
    case empty
}
func classify(_ s: Shape, _ strict: Bool) -> String {
    switch s {
    case .circle(let r) where r > 10: return "big circle"
    case .circle(let r): return "circle " + String(r)
    case .rect(let w, let h) where w == h: return "square " + String(w)
    case .named(let t, let label) where t.name == label: return "self-named " + label
    case .empty where strict: return "strict empty"
    case .rect(let w, _): return "rect " + String(w)
    case .named(_, let label): return "named " + label
    default: return "other"
    }
}
func trivial(_ n: Int?) -> Int {
    switch n {
    case .some(let v) where v < 0: return -v
    case .some(let v): return v
    case .none: return 0
    }
}
enum Level { case low(Int), high(Int) }
func level(_ l: Level) -> Int {
    switch l {
    case .low(let v) where v > 5: return 100
    case .high(let v) where v > 5: return 200
    case .low(let v): return v
    case .high(let v): return v + 1
    }
}
func sign(_ n: Int?) -> Int {
    switch n {
    case .some(let v) where v < 0: return -1
    case let v? where v == 0: return 0
    case .some: return 1
    case .none: return 99
    }
}
func label(_ s: String?, _ short: Bool) -> String {
    switch s {
    case let t? where t.count < 3 && short: return "short " + t
    case nil where short: return "nothing, briefly"
    case .some(let t): return "long " + t
    case _: return "nothing"
    }
}
func main() -> Int32 {
    var out: [String] = []
    var total = 0
    for i in 0..<400 {
        out.append(classify(.circle(i % 20), false))
        out.append(classify(.rect(i % 3, 1), true))
        out.append(classify(.named(Tag("t" + String(i % 2)), "t1"), false))
        out.append(classify(.empty, i % 2 == 0))
        total += sign(i % 3 - 1) + sign(nil) + trivial(i - 200)
        out.append(label(i % 2 == 0 ? "ab" + String(i) : "a", i % 4 < 2) + label(nil, i % 3 == 0))
    }
    print(out[0], out[1], out[2], out[3], out[4], out[55], out[56], out[57], out[58], out[59])
    print(level(.low(9)), level(.low(2)), level(.high(9)), level(.high(1)), out.count, total)
    return Int32(level(.high(3)))
}
