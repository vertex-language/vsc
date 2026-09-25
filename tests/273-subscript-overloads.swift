// Subscripts overloaded by argument type: the arguments pick, literals
// included, and inside an optional chain.
struct Node {
    var n: Int
    subscript(key: String) -> Node? { return key.isEmpty ? nil : Node(n: n + 1) }
    subscript(index: Int) -> Node? { return index < 0 ? nil : Node(n: n + 10) }
    subscript(scale: Double) -> Int { return Int(Double(n) * scale) }
}

let root = Node(n: 0)
print(root[1]!.n)
print(root["a"]!.n)
print(root["a"]?[2]?["b"]?.n ?? -1)
print(root[-1]?.n ?? -1)
print(root[""]?[0]?.n ?? -1)
print(root[3]![2.5])
let i = 4
print(root[i]!.n, root["k"]!.n)
