// A value whose type is not known until it arrives.
//
// A generic constrained by a protocol is specialized, so by the time
// its body runs the type is known and the implementation can be named
// -- see 137. An existential is the other half of that: `any Shape`
// is one type, a function taking it is lowered once, and which
// implementation runs is decided by the value rather than by the call
// site.
//
// The value carries the answer with it. `any Shape` is a buffer with
// the concrete value in it and, beside it, the table of that type's
// implementations of Shape. A call reads the row out of the table and
// applies it to the buffer, which is what makes Square and Triangle
// -- different sizes, different fields, different arithmetic --
// interchangeable here.
protocol Shape { func area() -> Int32 }

// Two conformers with nothing in common but the promise.
struct Square: Shape {
    var side: Int32
    func area() -> Int32 { return side * side }
}
struct Triangle: Shape {
    var base: Int32
    var height: Int32
    func area() -> Int32 { return base * height / 2 }
}

// A protocol named where a type is wanted is an existential of
// itself, and `any Shape` says the same thing the long way. Both
// spellings mangle to one symbol, so these are two names for one
// convention.
func measure(_ s: Shape) -> Int32 { return s.area() }
func measureAny(_ s: any Shape) -> Int32 { return s.area() }

// An existential parameter passed on is the same storage passed on,
// not five words copied into a register.
func twice(_ s: Shape) -> Int32 { return measure(s) + measureAny(s) }

func main() -> Int32 {
    if measure(Square(side: 5)) != 25 { return 91 }
    if measureAny(Triangle(base: 3, height: 4)) != 6 { return 92 }
    if twice(Square(side: 3)) != 18 { return 93 }
    return measure(Square(side: 4)) + measureAny(Triangle(base: 10, height: 2))
}
