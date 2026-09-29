// ObjectIdentifier as a dictionary key for class instances.
final class Node { let name: String; init(_ n: String) { name = n } }
let a = Node("a"), b = Node("b")
var seen: [ObjectIdentifier: Int] = [:]
for n in [a, b, a, a] { seen[ObjectIdentifier(n), default: 0] += 1 }
print(seen[ObjectIdentifier(a)]!, seen[ObjectIdentifier(b)]!, ObjectIdentifier(a) == ObjectIdentifier(b), seen.count)
