// Value generic parameters (SE-0452) and suppressed conformances.

struct Vector<let count: Int, Element> {
    var storage: [Element] = []
}

typealias Vec3 = Vector<3, Double>
typealias Vec1 = Vector<1, Int>

struct Buffer<let n: Int>: ~Copyable {
    deinit {}
}

// A suppressed conformance is a type, so a composition may hold more
// than one of them.
struct Pair<T: ~Copyable & ~Escapable>: ~Copyable & ~Escapable {}

protocol Container: ~Copyable {
    associatedtype Item: ~Copyable
}

func consume<T: ~Copyable>(_ x: consuming T) {}

func constrained<T>(_ x: borrowing T) where T: ~Copyable & ~Escapable {}

extension Buffer where Self: ~Copyable {}

// A generic parameter may be named Self.
func registerAll<Self>(_ x: Self) where Self: Equatable {}

struct Box<Self> {}
