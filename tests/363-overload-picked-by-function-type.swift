// An overloaded function named bare in a literal of function type takes the
// overload that type asks for.
func magnitude(_ x: Float) -> Float { x < 0 ? -x : x }
func magnitude(_ x: Double) -> Double { x < 0 ? -x * 10 : x * 10 }
let fs: [(String, (Double) -> Double)] = [("magnitude", magnitude)]
for (name, f) in fs { print(name, f(-2.5)) }
