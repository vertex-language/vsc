// A method's receiver is the value it was called on.
struct Rect {
    var w: Int32
    var h: Int32

    func area() -> Int32 { return w * h }
    func perimeter() -> Int32 { return (w + h) * 2 }
}

func main() -> Int32 {
    let r = Rect(w: 3, h: 4)
    return r.area() + r.perimeter()
}
