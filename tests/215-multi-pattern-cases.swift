// One case with several patterns that bind the same names.
enum Shape { case square(side: Int), rect(w: Int, h: Int), circle(r: Int), line(len: Int) }
func size(_ s: Shape) -> Int {
    switch s {
    case .square(let n), .circle(let n), .line(let n): return n
    case let .rect(w, h): return max(w, h)
    }
}
print([Shape.square(side: 2), .rect(w: 3, h: 7), .circle(r: 5), .line(len: 1)].map(size))
