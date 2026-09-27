// Protocol-extension methods on existentials, further: one calling
// another through self in a closure, a property requirement, and a
// class conformer beside a struct.
protocol Shape { var name: String { get }; func area() -> Double }
extension Shape {
    func describe() -> String { "\(name): \(area())" }
    func twice() -> String { [1, 2].map { _ in self.describe() }.joined(separator: " | ") }
    func scaled(_ k: Double) -> Double { let f = { area() * k }; return f() }
}
struct Sq: Shape { var s: Double; var name: String { "sq" }; func area() -> Double { s * s } }
final class Circ: Shape { let r: Double; init(r: Double) { self.r = r }; var name: String { "circ" }; func area() -> Double { 3 * r * r } }
let shapes: [any Shape] = [Sq(s: 2), Circ(r: 1)]
for s in shapes { print(s.describe(), s.twice(), s.scaled(2)) }
