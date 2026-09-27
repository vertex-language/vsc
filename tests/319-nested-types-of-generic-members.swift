// Types nested in a generic type, further: a nested struct's methods, one
// made inside the outer type's own methods, a nested enum carrying T, and
// one named through an instance from outside.
struct Stack<T> {
    struct Node { var value: T; var depth: Int
        func describe() -> String { "\(value)@\(depth)" } }
    enum State { case empty, top(T) }
    var nodes: [Node] = []
    mutating func push(_ v: T) { nodes.append(Node(value: v, depth: nodes.count)) }
    var state: State { nodes.isEmpty ? .empty : .top(nodes[nodes.count - 1].value) }
}
var s = Stack<String>()
s.push("a"); s.push("b")
print(s.nodes.map { $0.describe() })
switch s.state { case .empty: print("empty"); case .top(let v): print("top", v) }
let n = Stack<Double>.Node(value: 1.5, depth: 0)
print(n.describe())
