// Operators declared as static methods on a type of a program's own.
struct Vec: Equatable, CustomStringConvertible {
    var x, y: Int
    static func + (a: Vec, b: Vec) -> Vec { Vec(x: a.x + b.x, y: a.y + b.y) }
    static func * (k: Int, v: Vec) -> Vec { Vec(x: k * v.x, y: k * v.y) }
    static prefix func - (v: Vec) -> Vec { Vec(x: -v.x, y: -v.y) }
    static func += (a: inout Vec, b: Vec) { a = a + b }
    var description: String { "(\(x), \(y))" }
}
var v = Vec(x: 1, y: 2)
v += Vec(x: 10, y: 10)
print(v, -v, 3 * v, v + -v == Vec(x: 0, y: 0))
