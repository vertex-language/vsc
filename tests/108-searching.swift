// Searching an array.
let xs = [4, 8, 15, 16, 23, 42]
print(xs.contains(15), xs.contains(7), xs.contains { $0 > 40 })
print(xs.firstIndex(of: 16) as Any, xs.firstIndex(of: 1) as Any)
print(xs.first { $0 % 2 == 1 } as Any, xs.last { $0 < 20 } as Any, xs.lastIndex(where: { $0 < 10 }) as Any)
print(xs.allSatisfy { $0 > 0 }, xs.min() as Any, xs.max() as Any)
