// A type parameter inferred from the type the result is given.
func zero<T: Numeric>(_ op: Int32) -> T { T(exactly: op)! }
let a: Int = zero(3)
let b: Double = zero(4)
func f<T: Numeric>(_: T.Type) -> T { let v: T = zero(5); return v * v }
print(a, b, f(Int.self))
