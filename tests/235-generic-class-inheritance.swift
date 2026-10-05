// A generic class, a subclass that fixes its parameter, and overriding across them.
class Container<T> {
    var items: [T] = []
    func add(_ x: T) { items.append(x) }
    func summary() -> String { "\(items.count) items" }
}
class IntBag: Container<Int> {
    override func add(_ x: Int) { super.add(x * 2) }
    override func summary() -> String { super.summary() + ", total \(items.reduce(0, +))" }
}
class Named<T>: Container<T> {
    let name: String
    init(_ name: String) { self.name = name }
    override func summary() -> String { name + ": " + super.summary() }
}
let bags: [Container<Int>] = [Container(), IntBag(), Named("n")]
for b in bags { b.add(5); b.add(1); print(b.summary()) }
