// A protocol extension's method whose result is Self, called on an
// existential: the result is the existential again.
protocol Shape { var sides: Int { get } }
extension Shape {
    func itself() -> Self { self }
}
struct Square: Shape { let sides = 4 }
let s: any Shape = Square()
let t = s.itself()
print(t.sides, t is Square)
