// A value that isn't optional compared with nil. With no operator of its
// own it is promoted to an optional, which is never empty; with one, the
// nil is a value of its type, made by its literal initializer.
final class Node: ExpressibleByNilLiteral {
    var v = 0
    init() {}
    required init(nilLiteral: ()) { v = 9 }
}
func made() -> Node { print("made"); return Node() }
let n: Node = nil
print(n != nil, n == nil, nil == n, nil != n)
print(made() == nil)

struct Own: ExpressibleByNilLiteral {
    var v = 0
    init() {}
    init(nilLiteral: ()) { v = 1 }
    static func != (a: Own, b: Own) -> Bool { print("own !=", a.v, b.v); return a.v != b.v }
    static func == (a: Own, b: Own) -> Bool { print("own ==", a.v, b.v); return a.v == b.v }
}
print(Own() != nil, Own() == nil)

struct Derived: ExpressibleByNilLiteral, Equatable {
    var v = 0
    init() {}
    init(nilLiteral: ()) { v = 1 }
}
print(Derived() == nil, Derived() != nil, nil == Derived())

struct Plain: ExpressibleByNilLiteral {
    var v = 0
    init() {}
    init(nilLiteral: ()) { v = 1 }
}
print(Plain() == nil, Plain() != nil)
