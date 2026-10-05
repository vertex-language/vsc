// More of the sequence algorithms.
let xs = [3, 1, 4, 1, 5, 9, 2, 6]
print(xs.prefix(while: { $0 < 5 }), xs.drop(while: { $0 < 5 }))
print(xs.elementsEqual([3, 1, 4, 1, 5, 9, 2, 6]), xs.starts(with: [3, 1]))
print(xs.max(by: { $0 % 5 < $1 % 5 }) as Any, xs.split(separator: 1))
print(xs.reversed().first as Any, xs.count(where: { $0 > 4 }))
var ys = xs
ys.swapAt(0, 7)
print(ys, xs.lexicographicallyPrecedes(ys))
