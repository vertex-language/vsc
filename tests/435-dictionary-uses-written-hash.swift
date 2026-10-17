// A dictionary and a set call a key's written hash(into:) and ==, here
// ones that ignore a field, rather than comparing keys structurally.
struct Key: Hashable {
    let x: Int
    let tag: String
    static func == (a: Key, b: Key) -> Bool { a.x == b.x }
    func hash(into h: inout Hasher) { h.combine(x) }
}
var d: [Key: Int] = [:]
d[Key(x: 1, tag: "a")] = 10
d[Key(x: 1, tag: "b")] = 20
print(d.count, d[Key(x: 1, tag: "z")] ?? -1)
var s: Set<Key> = [Key(x: 2, tag: "p")]
print(s.insert(Key(x: 2, tag: "q")).inserted, s.count, s.contains(Key(x: 2, tag: "r")))
