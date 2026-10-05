// A single-expression body returns its value, and if and switch are expressions.
func square(_ x: Int) -> Int { x * x }
func sign(_ x: Int) -> String {
    if x < 0 { "minus" } else if x == 0 { "zero" } else { "plus" }
}
func word(_ x: Int) -> String {
    switch x {
    case 1: "one"
    case 2: "two"
    default: "many"
    }
}
let parity = if square(3) % 2 == 0 { "even" } else { "odd" }
print(square(9), sign(-2), sign(0), word(2), word(7), parity)
