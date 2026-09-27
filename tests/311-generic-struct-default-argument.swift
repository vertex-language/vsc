// A default argument that constructs a generic struct, and an optional of
// a generic struct.
struct Epi<T: Numeric> { var scale: T = 1 }
func apply<T: Numeric>(_ x: T, e: Epi<T> = Epi<T>()) -> T { x * e.scale }
print(apply(3), apply(2.5, e: Epi(scale: 2)))
func maybe<T: Numeric>(_ x: T, e: Epi<T>? = nil) -> T { x * (e?.scale ?? 1) }
print(maybe(4), maybe(4, e: Epi(scale: 3)))
