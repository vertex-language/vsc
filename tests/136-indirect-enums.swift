// An indirect enum can hold itself: an expression tree.
indirect enum Expr {
    case num(Int)
    case add(Expr, Expr)
    case mul(Expr, Expr)
    case neg(Expr)
}
func eval(_ e: Expr) -> Int {
    switch e {
    case .num(let n): return n
    case let .add(a, b): return eval(a) + eval(b)
    case let .mul(a, b): return eval(a) * eval(b)
    case .neg(let a): return -eval(a)
    }
}
let e = Expr.add(.num(2), .mul(.num(3), .neg(.num(4))))
print(eval(e))
