// A struct that holds itself through an array: a tree, walked recursively.
struct Tree {
    var value: Int
    var children: [Tree] = []
    var sum: Int { children.reduce(value) { $0 + $1.sum } }
    var depth: Int { 1 + (children.map(\.depth).max() ?? 0) }
    func render(_ indent: String = "") -> [String] {
        [indent + String(value)] + children.flatMap { $0.render(indent + "  ") }
    }
}
var t = Tree(value: 1, children: [Tree(value: 2), Tree(value: 3, children: [Tree(value: 4)])])
t.children[0].children.append(Tree(value: 5))
print(t.sum, t.depth)
print(t.render().joined(separator: "\n"))
