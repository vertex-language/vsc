// A struct stored in a class, read and written through it.
struct Point {
    var x: Int32
    var y: Int32
}

final class Node {
    var p: Point = Point(x: 1, y: 2)
    func total() -> Int32 { return p.x + p.y }
}

func main() -> Int32 {
    let n = Node()
    n.p = Point(x: 10, y: 20)
    let a = n.total()
    n.p.x = 12
    return a + n.total()
}
