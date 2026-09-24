// A precedence group places an operator relative to the standard ones.
precedencegroup PowerPrecedence {
    higherThan: MultiplicationPrecedence
    associativity: right
}
infix operator ^^: PowerPrecedence
func ^^ (b: Int, e: Int) -> Int { e == 0 ? 1 : b * (b ^^ (e - 1)) }
infix operator |>: AdditionPrecedence
func |> (x: Int, f: (Int) -> Int) -> Int { f(x) }
print(2 ^^ 3 ^^ 2, 2 * 3 ^^ 2, 1 + 2 |> { $0 * 10 })
