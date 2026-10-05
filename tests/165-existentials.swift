// any P holds any conforming value, with dispatch through the protocol.
protocol Shape { func area() -> Double }
struct Square: Shape { let side: Double; func area() -> Double { side * side } }
struct Circle: Shape { let r: Double; func area() -> Double { 3 * r * r } }
class Triangle: Shape {
    let b, h: Double
    init(_ b: Double, _ h: Double) { self.b = b; self.h = h }
    func area() -> Double { b * h / 2 }
}
let shapes: [any Shape] = [Square(side: 2), Circle(r: 1), Triangle(3, 4)]
for s in shapes { print(s.area()) }
var one: any Shape = Square(side: 5)
print(one.area())
one = Circle(r: 2)
print(one.area(), one is Circle, (one as? Square) == nil)
