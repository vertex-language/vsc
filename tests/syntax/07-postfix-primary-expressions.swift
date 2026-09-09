// §4.3 / §4.4 / §4.5 Postfix and Primary Expressions

struct Point {
    var x: Int
    var y: Int
    init(x: Int, y: Int) { self.x = x; self.y = y }
    subscript(index: Int) -> Int { index == 0 ? x : y }
}

// FunctionCallArgumentClause / TrailingClosures
func perform(_ work: () -> Void) { work() }
perform { print("trailing closure") }

func perform(times: Int, _ work: (Int) -> Void) { for i in 0..<times { work(i) } }
perform(times: 2) { i in print(i) }

// PostfixExpression InitializerArgumentClause: . init (...)
let point = Point.init(x: 1, y: 2)

// ExplicitMemberExpression: . Identifier
let pointX = point.x

// PostfixSelfExpression: . self
let pointType = Point.self

// SubscriptArgumentClause
let firstCoordinate = point[0]

// ForcedValueExpression: !
let optionalInt: Int? = 5
let forced = optionalInt!

// OptionalChainingExpression: ?
struct Wrapper { var inner: Point? }
let wrapper = Wrapper(inner: point)
let chained = wrapper.inner?.x

// ArrayLiteral / DictionaryLiteral
let arrayLiteral = [1, 2, 3,]                 // trailing comma allowed
let emptyDict: [String: Int] = [:]            // '[' : ']'
let dictLiteral = ["a": 1, "b": 2,]

// ImplicitMemberExpression: . Identifier
enum Direction { case north, south, east, west }
let heading: Direction = .north

// WildcardExpression
_ = point.x

// Literal expression macros
func debugLocation(file: String = #file, line: Int = #line, function: String = #function) {
    print(file, line, function)
}