// A class is shared by reference; === asks whether two are the same object.
class Node {
    var value: Int
    init(_ v: Int) { value = v }
}
let a = Node(1)
let b = a
let c = Node(1)
b.value = 99
print(a.value, a === b, a === c, a !== c)
func change(_ n: Node) { n.value = -1 }
change(c)
print(c.value)
