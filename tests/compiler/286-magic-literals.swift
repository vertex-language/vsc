// #line and #column are where they are written, as whatever integer type
// is wanted, and #function is the declaration around them spelled as Swift
// names it: labels, `_` for none, a getter by its property, a closure by
// the function it is in.

struct Point {
    var x: Int
    init(x: Int) { self.x = x }
    func move(_ dx: Int, by dy: Int) -> String { return #function }
    static func origin() -> String { #function }
    var name: String { #function }
    func made() -> String { return Point.madeBy }
    static let madeBy = "Point"
}

func free() -> String { #function }
func labeled(first a: Int, _ b: Int) -> String { #function }
func inClosure() -> String { let c = { #function }; return c() }
func nested() -> String { func inner() -> String { #function }; return inner() }

func check(_ ok: Bool, _ n: Int32) -> Int32 { return ok ? 0 : n }

func main() -> Int32 {
    var failed: Int32 = 0
    let line: Int32 = #line
    let column = #column
    // Lines are compared with each other: the file swiftc is given has a
    // line of its own added at the top.
    failed += check(column == 18 && #line == line + 4, 1)
    let p = Point(x: 1)
    failed += check(p.move(1, by: 2) == "move(_:by:)" && Point.origin() == "origin()", 2)
    failed += check(p.name == "name", 3)
    failed += check(free() == "free()" && labeled(first: 1, 2) == "labeled(first:_:)", 4)
    failed += check(inClosure() == "inClosure()" && nested() == "inner()", 5)
    print(column, #line - line, p.move(0, by: 0), p.name, labeled(first: 0, 0), nested())
    return failed
}
