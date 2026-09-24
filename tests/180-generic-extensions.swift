// Generic methods and subscripts, and extensions constrained on concrete types.
extension Sequence {
    func counted<K: Hashable>(by key: (Element) -> K) -> [K: Int] {
        var d: [K: Int] = [:]
        for e in self { d[key(e), default: 0] += 1 }
        return d
    }
}
extension Collection where Element == String {
    var totalLength: Int { reduce(0) { $0 + $1.count } }
}
struct Table {
    var cells: [String: Any] = ["n": 3, "s": "x"]
    subscript<T>(key: String, as type: T.Type) -> T? { cells[key] as? T }
}
let byLength = ["a", "bb", "cc", "ddd"].counted { $0.count }
print(byLength.sorted { $0.key < $1.key }.map { "\($0.key):\($0.value)" }, ["ab", "cde"].totalLength)
let t = Table()
print(t["n", as: Int.self] as Any, t["n", as: String.self] as Any, t["s", as: String.self] as Any)
