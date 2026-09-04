// §6.4 Functions

// FunctionName: Identifier | Operator
func greet(name: String) -> String { "Hello, \(name)" }

// ArgumentLabel variants: named, underscore (no label)
func move(from start: Point, to end: Point) { }
func add(_ a: Int, _ b: Int) -> Int { a + b }

struct Point { var x, y: Int }

// ParameterModifierList: inout / borrowing / consuming
func modify(_ value: inout Int) { value += 1 }
func inspect(_ value: borrowing [Int]) { print(value.count) }
func take(_ value: consuming [Int]) { print(value.count) }

// Variadic parameter: Type ...
func sum(_ numbers: Int...) -> Int { numbers.reduce(0, +) }

// GenericParameterClause + GenericWhereClause
func firstMatch<T: Equatable>(in array: [T], equalTo target: T) -> T? where T: Comparable {
    array.first { $0 == target }
}

// async / throws / rethrows in FunctionSignature
func fetchData() async throws -> Data { Data() }
func fetchDataCustomError() async throws(NetworkError) -> Data { Data() }
enum NetworkError: Error { case timeout }

func applyTwice(_ transform: (Int) throws -> Int, to value: Int) rethrows -> Int {
    try transform(try transform(value))
}

// FunctionDeclaration defining a custom operator function (FunctionName: Operator)
func + (lhs: Point, rhs: Point) -> Point {
    Point(x: lhs.x + rhs.x, y: lhs.y + rhs.y)
}