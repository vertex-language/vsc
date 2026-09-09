// A method that hands back a value of its own type.
struct Vec {
    var x: Int32
    var y: Int32

    func plus(_ o: Vec) -> Vec { return Vec(x: x + o.x, y: y + o.y) }
    func scaled(_ k: Int32) -> Vec { return Vec(x: x * k, y: y * k) }
    func total() -> Int32 { return x + y }
}

func main() -> Int32 {
    let a = Vec(x: 1, y: 2)
    let b = Vec(x: 3, y: 4)
    return a.plus(b).scaled(4).total() + 2
}
