// §4 Expressions

// TryOperator variants
enum SampleError: Error { case failed }
func mightThrow() throws -> Int { throw SampleError.failed }

func tryExamples() {
    if let value = try? mightThrow() { print(value) }
    // try! forces (would crash on failure, shown for grammar completeness)
    // let forced = try! mightThrow()
}

// AwaitOperator
func fetchValue() async -> Int { 42 }
func awaitExample() async {
    let value = await fetchValue()
    print(value)
}

// PrefixExpression: PrefixOperator PostfixExpression
let negated = -5
let notTrue = !true

// InOutExpression: & Expression
func increment(_ x: inout Int) { x += 1 }
var mutableValue = 10
increment(&mutableValue)

// consume / copy / borrow Expression
func consumeExample() {
    let a = [1, 2, 3]
    let b = consume a
    print(b)
}

// BinaryExpression variants
let sum = 1 + 2                       // BinaryOperator
var assignedValue = 0
assignedValue = 10                    // AssignmentOperator
let conditional = true ? 1 : 2        // ConditionalOperator

// TypeCastingOperator: is / as / as? / as!
let value: Any = 5
let isInt = value is Int
let asDouble = value as? Double
class Base {}
class Derived: Base {}
let derivedInstance: Base = Derived()
let forcedDowncast = derivedInstance as! Derived