// Iterating elements, indices, and enumerated pairs.
let xs = ["a", "b", "c"]
for x in xs { print(x, terminator: " ") }
print()
for i in xs.indices { print(i, xs[i], terminator: " ") }
print()
for (i, x) in xs.enumerated() { print("\(i):\(x)", terminator: " ") }
print()
for x in xs.reversed() { print(x, terminator: "") }
print()
xs.forEach { print($0.uppercased(), terminator: "") }
print()
