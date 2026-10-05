// §9 Generic Parameters and Arguments

// GenericParameterClause with TypeInheritanceClause
func maxValue<T: Comparable>(_ a: T, _ b: T) -> T {
    a > b ? a : b
}

// each TypeName (parameter packs)
struct Tuple<each Element> {
    var values: (repeat each Element)
}

// GenericArgumentClause: explicit specialization
let explicitArray = Array<String>()
let explicitDict = Dictionary<String, Int>()

// GenericWhereClause with ConformanceRequirement
func printAll<S: Sequence>(_ sequence: S) where S.Element: CustomStringConvertible {
    for element in sequence {
        print(element.description)
    }
}

// SameTypeRequirement
func zipEqual<A: Collection, B: Collection>(_ a: A, _ b: B) -> [(A.Element, B.Element)]
    where A.Element == B.Element {
    Array(zip(a, b))
}

// Negative conformance requirement: TypeIdentifier : ~ TypeIdentifier
struct NonCopyableBox<T> where T: ~Copyable {
    // illustrative only
}

// InheritanceItem with '~' (suppressing a conformance)
struct MoveOnly: ~Copyable {}