// A singly linked list of class nodes, freed in order when dropped.
final class Node {
    let value: Int
    var next: Node?
    init(_ v: Int, _ next: Node? = nil) { value = v; self.next = next }
    deinit { print("free", value) }
}
struct List: Sequence {
    var head: Node?
    mutating func push(_ v: Int) { head = Node(v, head) }
    func makeIterator() -> AnyIterator<Int> {
        var cur = head
        return AnyIterator {
            defer { cur = cur?.next }
            return cur?.value
        }
    }
}
var list = List()
for v in 1...4 { list.push(v) }
print(Array(list), list.reduce(0, +))
list.head = list.head?.next
print(Array(list))
list.head = nil
print("end")
