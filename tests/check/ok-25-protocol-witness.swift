protocol Named {
    var name: String { get }
}
struct Person: Named {
    var name: String
}
func greet(_ n: Named) -> String {
    return n.name
}
