protocol Shape {
    func area() -> Double
}
struct Circle: Shape {
    var r: Double
    func area() -> Double { return r * r }
}
struct Square: Shape {
    var s: Double
    func area() -> Double { return s * s }
}
func total(_ shapes: [Shape]) -> Double {
    var sum = 0.0
    for shape in shapes {
        sum += shape.area()
    }
    return sum
}
