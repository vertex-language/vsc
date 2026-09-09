enum Shape {
    case circle(radius: Double)
    case square(side: Double)
}
func area(_ s: Shape) -> Double {
    switch s {
    case .circle(let r): return r * r
    case .square(let side): return side * side
    }
}
