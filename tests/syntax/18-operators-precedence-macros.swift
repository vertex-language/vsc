// §6.9 Operators, Precedence Groups, and Macros

// PrecedenceGroupDeclaration
precedencegroup ExponentiationPrecedence {
    higherThan: MultiplicationPrecedence
    associativity: right
    assignment: false
}

// OperatorDeclaration referencing the custom precedence group
infix operator ^^: ExponentiationPrecedence

func ^^ (base: Double, power: Double) -> Double {
    var result = 1.0
    for _ in 0..<Int(power) { result *= base }
    return result
}

let raised = 2.0 ^^ 3.0 * 2.0   // exponent binds tighter than multiplication

// prefix/postfix operator declarations
prefix operator √
prefix func √(value: Double) -> Double { value.squareRoot() }
let root = √16.0

// MacroDeclaration (external macro expansion)
@freestanding(expression)
macro stringify<T>(_ value: T) -> (T, String) = #externalMacro(module: "MyMacros", type: "StringifyMacro")

// Using a macro expression (MacroExpansionExpression)
// let (value, text) = #stringify(1 + 2)