// §6.8 Extensions and Subscripts

struct Vector2D {
    var x: Double
    var y: Double
}

// ExtensionDeclaration adding conformance via TypeInheritanceClause
extension Vector2D: CustomStringConvertible {
    var description: String { "(\(x), \(y))" }
}

// Extension adding a computed property and subscript
extension Vector2D {
    var magnitude: Double { (x * x + y * y).squareRoot() }

    // SubscriptDeclaration with GetterSetterBlock
    subscript(component: Int) -> Double {
        get { component == 0 ? x : y }
        set {
            if component == 0 { x = newValue } else { y = newValue }
        }
    }
}

// Extension with GenericWhereClause (constrained extension)
extension Array where Element == Vector2D {
    var totalMagnitude: Double { reduce(0) { $0 + $1.magnitude } }
}

// SubscriptDeclaration with plain CodeBlock (read-only shorthand)
struct ReadOnlyGrid {
    let values: [[Int]]
    subscript(row: Int, col: Int) -> Int {
        values[row][col]
    }
}