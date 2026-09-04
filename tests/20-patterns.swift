// §8 Patterns

// WildcardPattern
func wildcardExample() {
    let (_, second) = (1, 2)
    print(second)
}

// IdentifierPattern with TypeAnnotation
let typedBinding: Int = 5

// ValueBindingPattern: var / let Pattern
func valueBindingExample(_ pair: (Int, Int)) {
    switch pair {
    case let (x, y):
        print(x, y)
    }
    switch pair {
    case var (x, y):
        x += 1
        print(x, y)
    }
}

// TuplePattern with labeled elements
let (a, b): (x: Int, y: Int) = (x: 1, y: 2)

// EnumCasePattern
enum Token { case number(Int), symbol(String) }
func matchToken(_ token: Token) {
    switch token {
    case .number(let value):
        print(value)
    case Token.symbol(let text):
        print(text)
    }
}

// OptionalPattern: IdentifierPattern ?
func optionalPatternExample(_ values: [Int?]) {
    for case let value? in values {
        print(value)
    }
}

// TypeCastingPattern: is Type / Pattern as Type
func typeCastingPatternExample(_ items: [Any]) {
    for item in items {
        switch item {
        case is String:
            print("a string")
        case let number as Int:
            print("an int: \(number)")
        default:
            print("something else")
        }
    }
}

// ExpressionPattern
func expressionPatternExample(_ value: Int) {
    switch value {
    case 0..<10:
        print("single digit")
    default:
        print("other")
    }
}