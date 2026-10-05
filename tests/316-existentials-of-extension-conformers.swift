// Existentials of values whose conformance is an extension's -- Int's, an
// Array's where its Element is Int -- and of instances of a generic type,
// each instance with a table of its own: methods and properties called
// through them.
protocol Shape { func area() -> Int; var name: String { get } }
extension Int: Shape { func area() -> Int { self * self }; var name: String { "int" } }
extension Array: Shape where Element == Int {
    func area() -> Int { reduce(0, +) }
    var name: String { "ints" }
}
struct Box<T>: Shape { var v: T; var k: Int
    func area() -> Int { k * 10 }
    var name: String { "box" } }
let shapes: [any Shape] = [3, [1, 2, 3], Box(v: "s", k: 2), Box(v: 1.5, k: 4)]
for s in shapes { print(s.name, s.area()) }
