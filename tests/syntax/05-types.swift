// §3 Types

// TypeIdentifier + GenericArgumentClause
let array: Array<Int> = [1, 2, 3]
let nested: Dictionary<String, [Int]> = ["a": [1, 2]]

// TypeIdentifier with member access (TypeName [GenericArgumentClause] . TypeIdentifier)
struct Outer { struct Inner { var x: Int } }
let inner: Outer.Inner = Outer.Inner(x: 1)

// ArrayType: '[' Type ']'
let numbers: [Int] = [1, 2, 3]

// DictionaryType: '[' Type : Type ']'
let scores: [String: Int] = ["Alice": 90, "Bob": 85]

// TupleType
let pair: (Int, String) = (1, "one")
let labeledTuple: (x: Int, y: Int) = (x: 0, y: 0)

// OptionalType: Type ?
let maybeNumber: Int? = nil

// ImplicitlyUnwrappedOptionalType: Type !
let forcedNumber: Int! = 5

// FunctionType
let adder: (Int, Int) -> Int = { $0 + $1 }
let asyncThrowing: () async throws -> Void = { }
let throwingFn: (() throws -> Void) -> Void = { _ in }

// MetatypeType: Type . TypeKeyword / Type . ProtocolKeyword
protocol Shape {}
let shapeMeta: Shape.Protocol = Shape.self
let intMeta: Int.Type = Int.self

// AnyType / SelfType
let anything: Any = 42
struct SelfDemo {
    static func make() -> Self { Self() }
}

// OpaqueType: some Type
func makeShape() -> some Shape { struct Circle: Shape {}; return Circle() }

// BoxedProtocolType: any Type
let boxed: any Shape = { struct Sq: Shape {}; return Sq() }()

// ProtocolCompositionType: TypeIdentifier {& TypeIdentifier}
protocol Named { var name: String { get } }
protocol Aged { var age: Int { get } }
func describe(_ value: Named & Aged) -> String { "\(value.name), \(value.age)" }

// Parenthesized Type
let parenthesized: (Int) = 5

// PackExpansionType / PackReferenceType (parameter packs)
func tuplify<each T>(_ value: repeat each T) -> (repeat each T) {
    return (repeat each value)
}

// MacroExpansionType