// What a view and a slice have through Sequence, and an array of tuples by
// its type's initializer.
let u = "b".unicodeScalars.first!
print("abc".unicodeScalars.contains(u), "xyz".unicodeScalars.contains(u))
print("abc".unicodeScalars.contains { $0.value > 98 })
print("héllo".utf8.contains(104), "abc".utf16.contains(120))
let a = [(Int, Int)](repeating: (-1, -1), count: 2)
print(a.count, a[1].0)
let named = [(x: Int, y: String)](repeating: (x: 1, y: "s"), count: 1)
print(named[0].y)
let words = ["a", "b", "c"]
print(words[0..<2].joined(separator: " "), words[1...].joined())
let nested = [[1], [2, 3], [4]]
print(Array(nested[0..<2].joined()))
