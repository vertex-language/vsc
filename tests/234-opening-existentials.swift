// An any P passed where a generic T: P is wanted is opened to its concrete type.
protocol Shape { var sides: Int { get } }
struct Tri: Shape { let sides = 3 }
struct Quad: Shape { let sides = 4 }
func describe<T: Shape>(_ s: T) -> String { "\(T.self) with \(s.sides)" }
func pairUp<T: Shape>(_ s: T) -> [T] { [s, s] }
let shapes: [any Shape] = [Tri(), Quad()]
for s in shapes {
    print(describe(s), pairUp(s).count)
}
