// §6.9 Right-Associative Precedence Groups and Advanced Math Operators

precedencegroup ExponentiationPrecedence {
    associativity: right
    higherThan: MultiplicationPrecedence
}

precedencegroup EquivalenceRelationPrecedence {
    associativity: none
    higherThan: ComparisonPrecedence
    lowerThan: AdditionPrecedence
}

infix operator ^^: ExponentiationPrecedence
infix operator ⊕: AdditionPrecedence
infix operator ⊗: MultiplicationPrecedence

func ^^(lhs: Double, rhs: Double) -> Double {
    var result = 1.0
    for _ in 0..<Int(rhs) {
        result *= lhs
    }
    return result
}

struct Matrix2x2 {
    var a: Double, b: Double
    var c: Double, d: Double
}

func ⊕(lhs: Matrix2x2, rhs: Matrix2x2) -> Matrix2x2 {
    return Matrix2x2(a: lhs.a + rhs.a, b: lhs.b + rhs.b, c: lhs.c + rhs.c, d: lhs.d + rhs.d)
}

func ⊗(lhs: Matrix2x2, rhs: Matrix2x2) -> Matrix2x2 {
    return Matrix2x2(
        a: lhs.a * rhs.a + lhs.b * rhs.c,
        b: lhs.a * rhs.b + lhs.b * rhs.d,
        c: lhs.c * rhs.a + lhs.d * rhs.c,
        d: lhs.c * rhs.b + lhs.d * rhs.d
    )
}

func testMathOperations() {
    let exp = 2.0 ^^ 3.0 ^^ 2.0
    let m1 = Matrix2x2(a: 1, b: 0, c: 0, d: 1)
    let m2 = Matrix2x2(a: 2, b: 3, c: 4, d: 5)
    let sum = m1 ⊕ m2
    let prod = m1 ⊗ m2
    _ = (exp, sum, prod)
}
