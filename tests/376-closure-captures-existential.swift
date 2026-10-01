// A closure that captures an existential held in memory -- a parameter, a
// let, a catch binding -- copies it into its context, as any value is.
protocol Shape {
    var name: String { get }
    func area() -> Int
}
struct Square: Shape {
    let side: Int
    var name: String { "square" }
    func area() -> Int { side * side }
}
final class Circle: Shape {
    let r: Int
    init(r: Int) { self.r = r }
    var name: String { "circle" }
    func area() -> Int { 3 * r * r }
    deinit { print("circle", r, "freed") }
}

func locked<R>(_ body: () throws -> R) rethrows -> R { try body() }

// A parameter, captured by a closure that does not escape.
func describe(_ s: any Shape) -> String { locked { "\(s.name) \(s.area())" } }
print(describe(Square(side: 3)))
print(describe(Circle(r: 2)))

// A let, captured by closures that outlive its scope.
func maker(_ s: any Shape) -> () -> Int {
    let t = s
    return { t.area() + 1 }
}
var fns: [() -> Int] = []
fns.append(maker(Square(side: 4)))
fns.append(maker(Circle(r: 1)))
for f in fns { print(f()) }
fns.removeAll()
print("cleared")

// Stored from inside the closure, beside a captured Int.
final class Holder {
    let s: any Shape
    init(_ s: any Shape) { self.s = s }
}
var slots: [Int: Holder] = [:]
func attach(_ s: any Shape, at i: Int) { locked { slots[i] = Holder(s) } }
attach(Square(side: 5), at: 1)
attach(Circle(r: 3), at: 2)
print(slots[1]!.s.area(), slots[2]!.s.name)
slots = [:]

// A class-bound existential, and two existentials in one closure.
protocol Line: AnyObject { func set(_ level: Bool) }
final class Pin: Line {
    let n: Int
    init(_ n: Int) { self.n = n }
    func set(_ level: Bool) { print("pin", n, level) }
}
func connect(_ a: any Line, _ b: any Line, _ level: Bool) {
    locked {
        a.set(level)
        b.set(!level)
    }
}
connect(Pin(1), Pin(2), true)

// Captured again by a closure inside the closure.
func twice(_ s: any Shape) -> Int {
    let f = { () -> Int in
        let g = { s.area() }
        return g() + g()
    }
    return f()
}
print(twice(Square(side: 2)))

// A throwing closure, and an Any.
func check(_ s: any Shape, limit: Int) throws -> Int {
    struct TooBig: Error {}
    return try locked {
        if s.area() > limit { throw TooBig() }
        return s.area()
    }
}
print((try? check(Square(side: 2), limit: 10)) ?? -1, (try? check(Square(side: 9), limit: 10)) ?? -1)
func show(_ x: Any) -> String { locked { "\(x)" } }
print(show(42), show("text"), show(Square(side: 1).side))
// A catch block's implicit error: an existential in memory too.
struct Failed: Error { let code: Int }
do {
    throw Failed(code: 7)
} catch {
    let describe = { "caught \(error)" }
    print(locked { describe() })
}
print("done")
