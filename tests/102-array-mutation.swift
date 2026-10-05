// Inserting, removing and replacing elements of an Array.
var xs = [1, 2, 3, 4, 5]
xs.insert(0, at: 0)
xs.remove(at: 2)
let last = xs.removeLast()
let first = xs.removeFirst()
xs.append(contentsOf: [7, 8])
xs.replaceSubrange(1..<3, with: [9, 9, 9])
print(xs, last, first)
xs.removeAll { $0 == 9 }
print(xs, xs.popLast() as Any, xs)
xs.removeAll()
print(xs, xs.popLast() as Any)
