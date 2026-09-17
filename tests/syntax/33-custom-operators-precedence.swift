// §6.9 Custom Unicode Operators and Precedence Groups

precedencegroup LogicalEquivalencePrecedence {
    associativity: left
    higherThan: ComparisonPrecedence
    lowerThan: NilCoalescingPrecedence
    assignment: false
}

precedencegroup VectorMultiplicationPrecedence {
    associativity: left
    higherThan: AdditionPrecedence
    lowerThan: MultiplicationPrecedence
}

infix operator ∧: LogicalConjunctionPrecedence
infix operator ∨: LogicalDisjunctionPrecedence
prefix operator ¬
infix operator ≈: ComparisonPrecedence
infix operator ×: VectorMultiplicationPrecedence

prefix func ¬(value: Bool) -> Bool {
    return !value
}

func ∧(lhs: Bool, rhs: Bool) -> Bool {
    return lhs && rhs
}

func ∨(lhs: Bool, rhs: Bool) -> Bool {
    return lhs || rhs
}

func ≈(lhs: Double, rhs: Double) -> Bool {
    return abs(lhs - rhs) < 0.0001
}

struct Vec2 {
    var x: Double
    var y: Double
}

func ×(lhs: Vec2, rhs: Vec2) -> Double {
    return lhs.x * rhs.y - lhs.y * rhs.x
}

func testOperators() {
    let condition = ¬false ∧ (true ∨ false)
    let approx = (1.00001 ≈ 1.0)
    let v1 = Vec2(x: 1, y: 0)
    let v2 = Vec2(x: 0, y: 1)
    let cross = v1 × v2
    _ = (condition, approx, cross)
}
