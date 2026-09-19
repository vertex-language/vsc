// Patterns nested inside patterns: a case inside a payload (`.node(.x, let
// n)`), an optional pattern over an expression (`true?`), a case under an
// optional pattern (`.c(let i)?`), and tuples of these -- in a switch, an
// `if case`, a `guard case` and a `for case`. Payloads that own things
// are let go of whether the pattern matches or not, and `var` binds
// storage of its own. Optional's cases spelled by name: `.some(x)`,
// `.none`, `Optional(x)`, `Optional<T>.none`. A Bool? is one byte.

final class Obj { let id: Int; init(_ id: Int) { self.id = id } }
enum Inner { case x, y }
enum Tree { case leaf, node(Inner, Int) }
enum Code { case c(Int), quit }
enum Name { case short(String), long(String, String), anon }
indirect enum Item { case named(Name, Int), boxed(Obj), pair(Item?, Int), nothing }
indirect enum Expr { case lit(Int), add(Expr, Expr), neg(Expr) }

func classify(_ t: Tree) -> String {
    switch t {
    case .leaf: return "leaf"
    case .node(.x, let n): return "x\(n)"
    case .node(.y, 0): return "y0"
    case .node(.y, let n): return "y\(n)"
    }
}

func pair(_ p: (Bool?, Int)) -> String {
    switch p {
    case (true?, _): return "t"
    case (false?, let n): return "f\(n)"
    case (nil, _): return "nil"
    }
}

func opt(_ c: Code?) -> Int {
    switch c {
    case .c(let i)?: return i
    case .quit?: return -1
    case nil: return -2
    }
}

func show(_ i: Item) -> String {
    switch i {
    case .named(.short(let s), let n) where n > 10: return "big short \(s) \(n)"
    case .named(.short("x"), _): return "x!"
    case .named(.short(let s), var n):
        n += 1
        return "short \(s) \(n)"
    case .named(.long(let a, let b), 0): return "long0 \(a)\(b)"
    case .named(.long(let a, _), let n): return "long \(a) \(n)"
    case .named(.anon, let n): return "anon \(n)"
    case .boxed(let o): return "obj \(o.id)"
    case .pair(.some(.boxed(let o)), let n): return "pair obj \(o.id) \(n)"
    case .pair(nil, let n): return "pair nil \(n)"
    case .pair(.named(.anon, _)?, _): return "pair anon"
    case .pair(_, let n): return "pair other \(n)"
    case .nothing: return "none"
    }
}

func eval(_ e: Expr) -> Int {
    switch e {
    case .lit(let n): return n
    case .add(.lit(let a), .lit(let b)): return a + b
    case .add(let a, let b): return eval(a) + eval(b)
    case .neg(.neg(let inner)): return eval(inner)
    case .neg(let inner): return -eval(inner)
    }
}

func tup(_ t: (String?, Item, Bool)) -> String {
    switch t {
    case ("a"?, .nothing, true): return "A"
    case (let s?, .boxed(let o), _): return "\(s)\(o.id)"
    case (nil, .named(.anon, let n), false): return "anon\(n)"
    case (_, _, let b): return "other \(b)"
    }
}

func main() -> Int32 {
    print(classify(.leaf), classify(.node(.x, 3)), classify(.node(.y, 0)), classify(.node(.y, 7)))
    print(pair((true, 1)), pair((false, 2)), pair((nil, 3)))
    print(opt(.c(5)), opt(.quit), opt(nil))
    if case .node(.x, let n) = Tree.node(.x, 4) { print(n) }
    if case (true?, let n) = (Optional(true), 8) { print(n) }
    if case .c(let i)? = Optional(Code.c(6)) { print(i) }

    let items: [Item] = [
        .named(.short("s"), 20), .named(.short("x"), 1), .named(.short("y"), 2),
        .named(.long("l", "m"), 0), .named(.long("l", "m"), 3), .named(.anon, 4),
        .boxed(Obj(7)), .pair(.boxed(Obj(8)), 9), .pair(nil, 10),
        .pair(.named(.anon, 0), 11), .pair(.nothing, 12), .nothing,
    ]
    for i in items { print(show(i)) }
    print(eval(.add(.lit(1), .lit(2))), eval(.add(.neg(.lit(3)), .lit(4))), eval(.neg(.neg(.lit(5)))))
    print(tup(("a", .nothing, true)), tup(("b", .boxed(Obj(1)), false)), tup((nil, .named(.anon, 3), false)), tup((nil, .nothing, true)))
    for case .named(.short(let s), let n) in items { print("short:", s, n) }
    for case .pair(.boxed(let o)?, _) in items { print("boxed in pair:", o.id) }
    for case .long(let a, var b) in [Name.short("s"), .long("a", "b")] { b += "!"; print(a, b) }
    if case .named(.long(let a, let b), let n) = items[3], n == 0 { print(a, b) }
    if case (let s?, .named(.anon, _), _) = ("q" as String?, Item.named(.anon, 1), true) { print(s) }
    guard case .neg(.lit(let n)) = Expr.neg(.lit(6)) else { return 1 }
    print(n)
    var count = 0
    for i in items {
        if case .named(_, var n) = i { n *= 2; count += n }
    }
    print(count)

    let a: Int? = .some(1)
    let b = Optional.some(2)
    let c: Int? = .none
    let d = Optional<Int>.none
    let e = Optional<Int>(3)
    let f = Optional(4)
    print(a!, b!, c == nil, d == nil, e!, f!)
    var flag: Bool? = true
    print(flag == true, flag == nil)
    flag = nil
    print(flag == true, flag == nil, flag ?? false)
    return Int32(count)
}
