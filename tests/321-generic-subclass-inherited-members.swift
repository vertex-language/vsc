// A generic subclass of a generic class: the superclass's property
// defaults and its methods called straight on the subclass, whose
// parameters stand in for the superclass's.
class Container<T> {
    var items: [T] = []
    func add(_ x: T) { items.append(x) }
    func first() -> T? { items.first }
}
class Named<T>: Container<T> {
    let name: String
    init(_ name: String) { self.name = name }
}
class Pair<A, B>: Container<B> {
    var a: A
    init(_ a: A) { self.a = a }
}
let n = Named<Int>("n"); n.add(1); n.add(2); print(n.items.count, n.first()!)
let s = Named<String>("s"); s.add("x"); print(s.first()!, s.name)
let p = Pair<String, Int>("k"); p.add(7); print(p.a, p.first()!, p.items)
