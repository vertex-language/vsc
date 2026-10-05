// An associated type is a placeholder a conforming type fills in.
protocol Queue {
    associatedtype Element
    mutating func push(_ e: Element)
    mutating func pop() -> Element?
    var isEmpty: Bool { get }
}
struct FIFO<T>: Queue {
    private var items: [T] = []
    mutating func push(_ e: T) { items.append(e) }
    mutating func pop() -> T? { items.isEmpty ? nil : items.removeFirst() }
    var isEmpty: Bool { items.isEmpty }
}
func drain<Q: Queue>(_ q: inout Q) -> [Q.Element] {
    var out: [Q.Element] = []
    while let e = q.pop() { out.append(e) }
    return out
}
var q = FIFO<String>()
q.push("a"); q.push("b"); q.push("c")
print(drain(&q), q.isEmpty)
