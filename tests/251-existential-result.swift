// A function whose result is an existential, from a return and from a ternary of two types.
protocol Shape { func area() -> Int }
struct Square: Shape { var side: Int; func area() -> Int { side * side } }
struct Rect: Shape { var w: Int; var h: Int; func area() -> Int { w * h } }
func make(_ n: Int) -> any Shape { return Square(side: n) }
func pick(_ wide: Bool) -> any Shape { wide ? Rect(w: 4, h: 2) : Square(side: 3) }
print(make(6).area(), pick(true).area(), pick(false).area())
