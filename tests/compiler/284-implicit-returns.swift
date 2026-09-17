// A body that is one expression returns it without saying so: a function,
// a method, a static method, a protocol's witness reached directly, through
// a generic and through an existential, a computed property, a getter, and
// a nested function. A body of Void type runs its expression and returns
// nothing.

protocol Shape {
    func area() -> Int
}

struct Square: Shape {
    var side: Int
    func area() -> Int { side * side }
    var perimeter: Int { 4 * side }
    static func unit() -> Square { Square(side: 1) }
}

final class Counter {
    var n = 0
    var doubled: Int { get { n * 2 } }
    func bump() { n += 1 }
    func next() -> Int { n + 1 }
}

func square(_ x: Int) -> Int { x * x }
func name(_ ok: Bool) -> String { ok ? "yes" : "no" }
func total<S: Shape>(_ s: S) -> Int { s.area() + 1 }
func boxed(_ s: any Shape) -> Int { s.area() }

func outer(_ x: Int) -> Int {
    func inner(_ y: Int) -> Int { y + 1 }
    return inner(x) * 2
}

func check(_ ok: Bool, _ n: Int32) -> Int32 { return ok ? 0 : n }

func main() -> Int32 {
    var failed: Int32 = 0
    let s = Square(side: 3)
    failed += check(square(4) == 16 && name(true) == "yes" && name(false) == "no", 1)
    failed += check(s.area() == 9 && s.perimeter == 12 && Square.unit().area() == 1, 2)
    failed += check(total(s) == 10 && boxed(s) == 9, 3)
    let c = Counter()
    c.bump()
    c.bump()
    failed += check(c.doubled == 4 && c.next() == 3, 4)
    failed += check(outer(4) == 10, 5)
    print(square(4), name(false), s.area(), s.perimeter, total(s), boxed(s), c.doubled, outer(4))
    return failed
}
