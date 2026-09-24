// Cases carry values of their own, bound out by pattern.
enum Shape {
    case circle(radius: Double)
    case rect(width: Double, height: Double)
    case point
}
func area(_ s: Shape) -> Double {
    switch s {
    case .circle(let r): return 3 * r * r
    case let .rect(w, h): return w * h
    case .point: return 0
    }
}
let shapes: [Shape] = [.circle(radius: 2), .rect(width: 3, height: 4), .point]
for s in shapes { print(area(s)) }
if case .rect(let w, _) = shapes[1] { print("width", w) }
