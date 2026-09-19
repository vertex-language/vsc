// Interpolating and printing enums that carry values -- a struct, a
// tuple, a box, an array, an optional, an indirect case -- and tuples,
// labelled or not, nested, inside arrays and dictionaries. Also: a
// static stored property written by name and through its type; a value
// discarded with `let _ =` is still computed; a tuple returned or held
// in memory is taken apart into names; a parameter named like a type
// with that type's constructor as its default.

struct Pt { var x: Int; var y: Int }
final class Obj { var n = 1 }
enum EventResult { case handled, ignored(String), moved(Pt, Int), boxed(Obj), many([Int]), opt(Int?) }
enum Wrapper { case inner(EventResult), pair(Int, String) }
indirect enum Tree { case leaf(Int), node(Tree, Tree) }
enum Gen<T> { case some(T), none }
struct Rect { var x: Float; var y: Float; var w: Float; var h: Float }
struct Modifiers { var shift = false; var ctrl = false }
struct Event {
    var key: Int
    var mods: Modifiers
    init(key: Int, Modifiers: Modifiers = Modifiers()) { self.key = key; self.mods = Modifiers }
    func with(Modifiers: Modifiers = Modifiers(shift: true)) -> Event { return Event(key: key, Modifiers: Modifiers) }
}
func make(Modifiers: Modifiers = Modifiers()) -> Modifiers { return Modifiers }

final class Counter {
    var n = 0
    func bump() -> Int { n += 1; return n }
    func note() -> String { n += 10; return "noted" }
    static var made = 0
    static func make() -> Counter { made += 1; return Counter() }
}
struct Box {
    var n = 0
    mutating func bump() -> Int { n += 1; return n }
    func peek() -> Bool { print("peek"); return true }
}
var global = 0
func side() -> Int { global += 1; return global }

struct Wide { var a: Int; var b: Int; var c: Int; var d: Int; var e: Int }
enum Err: Error { case bad }
func pair() -> (Int, String) { return (1, "one") }
func wide() -> (Wide, Int) { return (Wide(a: 1, b: 2, c: 3, d: 4, e: 5), 9) }
func triple() -> (Int, (String, Bool), [Int]) { return (2, ("two", true), [2, 2]) }
func getPair(_ ok: Bool) throws -> (Int, Int) { if !ok { throw Err.bad }; return (3, 4) }

func main() -> Int32 {
    let rs: [EventResult] = [.handled, .ignored("why"), .moved(Pt(x: 1, y: 2), 3), .many([1, 2]), .opt(nil), .opt(4)]
    for r in rs { print("\(r)") }
    print(rs)
    let w = Wrapper.inner(.moved(Pt(x: 0, y: 0), 1))
    print("\(w) \(Wrapper.pair(1, "s"))")
    let o: EventResult? = .ignored("x")
    print("\(o as Any)", o as Any)
    let t = Tree.node(.leaf(1), .node(.leaf(2), .leaf(3)))
    print("\(t)")
    let g: Gen<Int> = .some(5)
    print("\(g)")
    let rect = Rect(x: 1, y: 2, w: 3.5, h: 4)
    let named: (x: Float, y: Float) = (1.5, 2)
    let plain = (1, "two", true)
    print("\(rect)")
    print("\(named) \(plain)")
    print(named, plain)
    let n: (Int, (String, Bool)) = (1, ("a", true))
    let arr: (a: [Int], b: Int?) = ([1, 2], nil)
    print("\(n) \(arr)")
    let ts: [(Int, String)] = [(1, "a"), (2, "b")]
    print(ts, "\(ts)")
    let d: [String: (Int, Int)] = ["k": (1, 2)]
    print(d)

    let e = Event(key: 7)
    print(e.key, e.mods.shift, e.with().mods.shift, make().ctrl)

    let c = Counter()
    let _ = c.bump()
    _ = c.bump()
    let _ = c.note()
    c.bump()
    print(c.n)
    var b = Box()
    let _ = b.bump()
    _ = b.bump()
    let _ = b.peek()
    print(b.n)
    let _ = side()
    _ = side()
    Counter.made = 5
    let _ = Counter.make()
    print(global, Counter.made)

    var (p, q) = pair()
    p += 1; q += "!"
    print(p, q)
    let (wd, k) = wide()
    print(wd.e, k)
    let (u, (s, f), xs) = triple()
    print(u, s, f, xs)
    do {
        let (x, y) = try getPair(true)
        print(x, y)
        let (_, z) = try getPair(true)
        print(z)
    } catch { print("caught") }
    let stored = wide()
    let (w2, n2) = stored
    print(w2.a, n2)
    for (i, v) in ts { print(i, v) }
    return Int32(c.n)
}
