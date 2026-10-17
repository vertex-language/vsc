// An array of existentials walked with for-in, flatMap, map and count, and
// handed to a generic function.
protocol Node { func render() -> [String] }
struct Text: Node {
    let s: String
    func render() -> [String] { [s] }
}
struct Group: Node {
    let kids: [any Node]
    func render() -> [String] { kids.flatMap { $0.render() } }
}
func count<T>(_ xs: [T]) -> Int { xs.count }
let tree: [any Node] = [Text(s: "a"), Group(kids: [Text(s: "b"), Text(s: "c")])]
for n in tree { print(n.render()) }
print(tree.flatMap { $0.render() }, tree.count, count(tree))
print(tree.map { $0.render().count })
