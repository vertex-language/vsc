// An indirect case keeps what it carries in a box of its own, which is
// what lets an enum hold itself: a whole `indirect enum`, or one
// `indirect case` beside cases carried in place, generic or not, with
// counted payloads that a switch binds, drops, or hands to a default.

indirect enum Tree {
    case leaf(Int)
    case node(Tree, Tree)
}

func sum(_ t: Tree) -> Int {
    switch t {
    case .leaf(let v): return v
    case .node(let a, let b): return sum(a) + sum(b)
    }
}

func depth(_ t: Tree) -> Int {
    switch t {
    case .leaf: return 1
    case .node(let a, let b):
        let l = depth(a), r = depth(b)
        return 1 + (l > r ? l : r)
    }
}

func build(_ n: Int) -> Tree {
    if n == 0 {
        return .leaf(1)
    }
    return .node(build(n - 1), .leaf(n))
}

enum Expr {
    case num(Int)
    case name(String)
    indirect case add(Expr, Expr)
    indirect case neg(Expr)
}

func show(_ e: Expr) -> String {
    switch e {
    case .num(let n): return String(n)
    case .name(let s): return s
    case .add(let a, let b): return "(" + show(a) + " + " + show(b) + ")"
    case .neg(let x): return "-" + show(x)
    }
}

func eval(_ e: Expr) -> Int {
    switch e {
    case .num(let n): return n
    case .add(let a, let b): return eval(a) + eval(b)
    case .neg(let x): return -eval(x)
    default: return 0
    }
}

func isNegation(_ e: Expr) -> Bool {
    switch e {
    case .neg: return true
    case .add(_, _), .num(_): return false
    default: return false
    }
}

indirect enum List<T> {
    case empty
    case cons(T, List<T>)
}

func count(_ l: List<String>) -> Int {
    switch l {
    case .empty: return 0
    case .cons(_, let rest): return 1 + count(rest)
    }
}

func joined(_ l: List<String>) -> String {
    if case .cons(let head, let rest) = l {
        return head + joined(rest)
    }
    return ""
}

indirect enum Path: Equatable {
    case end(String)
    case step(String, Path)
}

struct Holder {
    var path: Path
    var n: Int
}

func steps(_ p: Path) -> Int {
    switch p {
    case .end: return 0
    case .step(_, let rest): return 1 + steps(rest)
    }
}

func check(_ ok: Bool, _ n: Int32) -> Int32 { return ok ? 0 : n }

func main() -> Int32 {
    var failed: Int32 = 0
    let t = Tree.node(.leaf(1), .node(.leaf(2), .leaf(3)))
    failed += check(sum(t) == 6 && depth(t) == 3, 1)

    var e = Expr.add(.num(2), .neg(.name("x")))
    failed += check(show(e) == "(2 + -x)" && eval(e) == 2, 2)
    failed += check(isNegation(.neg(.num(1))) && !isNegation(e), 3)
    e = .neg(e)
    failed += check(show(e) == "-(2 + -x)", 4)

    let l = List.cons("a", .cons("b", .cons("c", .empty)))
    failed += check(count(l) == 3 && joined(l) == "abc", 5)

    // Held in a struct, an optional and an array, reassigned in place,
    // captured, and compared by a derived ==.
    let p = Path.step("a", .end("b"))
    var h = Holder(path: p, n: 1)
    h.path = .step("c", h.path)
    let o: Path? = h.path
    let paths = [p, h.path, .end("d")]
    var n = 0
    for q in paths { n += steps(q) }
    let f = { () -> Int in steps(p) }
    failed += check(steps(h.path) == 2 && o != nil && n == 3 && f() == 1, 7)
    failed += check(p == Path.step("a", .end("b")) && p != h.path, 8)

    // Many times over, so that a box leaked or released twice on some
    // path shows up as a crash rather than hiding in one pass.
    var total = 0
    for i in 0..<2000 {
        let big = build(i % 20)
        total += sum(big) + depth(big)
        let x = Expr.add(.name("n" + String(i)), .neg(.num(i)))
        total += eval(x) + show(x).count
        total += count(.cons(String(i), .cons("z", .empty)))
    }
    failed += check(total == -1811220, 6)

    print(sum(t), show(e), joined(l), total)
    return failed
}
