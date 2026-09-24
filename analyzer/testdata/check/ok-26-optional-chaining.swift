class Node {
    var next: Node?
    var value = 0
}
func use(_ n: Node) -> Int {
    return n.next?.value ?? 0
}
