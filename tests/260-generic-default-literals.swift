// A default argument written for a type parameter takes the type the call
// gives it: `by k: T = 2` is a Float at a call with Float, an Int32 at one
// with Int32.
func scaled<T: Numeric>(_ x: T, by k: T = 2) -> T { return x * k }

func offset<T: BinaryFloatingPoint>(_ x: T, _ d: T = 0.25) -> T { return x + d }

print(scaled(Float(1.5)), scaled(Int32(4)), scaled(3.0, by: 3))
print(offset(Float(1)), offset(Double(2)))
