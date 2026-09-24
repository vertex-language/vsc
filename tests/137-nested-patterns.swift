// Patterns inside patterns: enums in tuples, optionals, and value guards.
enum Token { case num(Int), op(Character), end }
let tokens: [(Int, Token?)] = [(0, .num(7)), (1, .op("+")), (2, nil), (3, .num(-1)), (4, .end)]
for t in tokens {
    switch t {
    case (let i, .some(.num(let n))) where n < 0: print(i, "negative", n)
    case (let i, .num(let n)?): print(i, "number", n)
    case (_, .op(let c)?): print("operator", c)
    case (let i, nil): print(i, "missing")
    case (_, .end?): print("end")
    }
}
