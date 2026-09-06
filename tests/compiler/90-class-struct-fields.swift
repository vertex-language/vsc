// A class whose stored properties are structs, and a subclass adding more.
struct Point {
    var x: Int32
    var y: Int32
}

class Shape {
    var origin: Point = Point(x: 1, y: 2)
}

class Circle: Shape {
    var radius: Int32 = 3
}

func main() -> Int32 {
    let c = Circle()
    c.origin = Point(x: 10, y: 20)
    c.radius = 12
    if c.origin.x != 10 { return 91 }
    if c.origin.y != 20 { return 92 }
    if c.radius != 12 { return 93 }
    return c.origin.x + c.origin.y + c.radius
}
