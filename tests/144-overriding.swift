// override replaces a method and property; super reaches the one it replaced.
class Shape {
    var name: String { "shape" }
    func area() -> Double { 0 }
    func describe() -> String { "\(name) with area \(area())" }
}
class Square: Shape {
    let side: Double
    init(side: Double) { self.side = side }
    override var name: String { "square" }
    override func area() -> Double { side * side }
}
class LabelledSquare: Square {
    override func describe() -> String { "[" + super.describe() + "]" }
}
let shapes: [Shape] = [Shape(), Square(side: 3), LabelledSquare(side: 2)]
for s in shapes { print(s.describe()) }
