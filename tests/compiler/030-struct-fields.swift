// A struct is its fields, laid out in order.
struct Point {
    var x: Int32
    var y: Int32
}

func main() -> Int32 {
    let p = Point(x: 3, y: 4)
    return p.x * 10 + p.y
}
