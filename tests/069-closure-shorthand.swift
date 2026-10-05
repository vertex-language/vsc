// Inferred types, $0, trailing closures, and operators as closures.
let xs = [5, 3, 9, 1]
print(xs.map { $0 * 10 })
print(xs.sorted(by: >))
print(xs.sorted { a, b in a < b })
print(xs.reduce(0, +))
print(xs.filter { $0 > 2 }.count)
