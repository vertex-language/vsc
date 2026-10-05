// The default subscript, and building dictionaries by grouping and uniquing.
var counts: [Character: Int] = [:]
for c in "mississippi" { counts[c, default: 0] += 1 }
for k in counts.keys.sorted() { print(k, counts[k]!) }
let groups = Dictionary(grouping: ["apple", "avocado", "banana", "blueberry", "cherry"]) { $0.first! }
for k in groups.keys.sorted() { print(k, groups[k]!) }
let merged = Dictionary([("a", 1), ("b", 2), ("a", 3)], uniquingKeysWith: +)
print(merged.sorted { $0.key < $1.key }.map { "\($0.key)=\($0.value)" })
