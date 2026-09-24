// §9.1 / §9.2 Complex Generics and Parameter Packs

protocol Shape {
    associatedtype Edge
    func perimeter() -> Double
}

protocol Polygon: Shape where Edge: Comparable {
    var vertexCount: Int { get }
}

struct Triangle<T: Comparable>: Polygon {
    typealias Edge = T
    var a: T, b: T, c: T
    var vertexCount: Int { 3 }
    func perimeter() -> Double { 0.0 }
}

// Multi-pack generics with constraints
func pairPacks<each First, each Second>(
    first: repeat each First,
    second: repeat each Second
) -> (repeat (each First, each Second)) {
    return (repeat (each first, each second))
}

// Multiple where clauses on nested functions
func processElements<C: Collection>(collection: C) where C.Element: Hashable {
    func inner<S: Sequence>(seq: S) -> [C.Element] where S.Element == C.Element {
        var seen = Set<C.Element>()
        var out = [C.Element]()
        for item in seq {
            if seen.insert(item).inserted {
                out.append(item)
            }
        }
        return out
    }
    _ = inner(seq: collection)
}

// Suppressed conformances
struct MoveOnlyData: ~Copyable {
    var buffer: Int
}
