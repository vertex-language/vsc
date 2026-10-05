// §2.4 Operators

// OperatorHead {OperatorCharacter}
prefix operator +++
postfix operator °
infix operator **: MultiplicationPrecedence
infix operator <->: ComparisonPrecedence

prefix func +++(value: Int) -> Int { value * 2 }
postfix func °(value: Double) -> String { "\(value) degrees" }

func **(base: Double, exponent: Double) -> Double {
    var result = 1.0
    for _ in 0..<Int(exponent) { result *= base }
    return result
}

func <->(lhs: Int, rhs: Int) -> Bool { lhs == rhs }

// Operator: . OperatorCharacter {OperatorCharacter}
infix operator .+.: AdditionPrecedence
func .+.(lhs: Int, rhs: Int) -> Int { lhs + rhs }

let doubled = +++5
let angle = 90.0°
let power = 2.0 ** 8.0
let equalCheck = 3 <-> 3