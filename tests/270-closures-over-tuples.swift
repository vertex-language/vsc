// Closures whose parameter is a tuple of an integer and a double: called
// directly, mapped over an array of such tuples, and used to index into
// another array -- each element in its own register, as a call passes it.
let logits: [Float] = [0.5, 1.5, 2.5]
let top: [(Int, Double)] = [(2, 2.4), (0, 0.4)]
let first: ((Int, Double)) -> Int = { $0.0 }
print(first((2, 2.4)), first(top[1]))
print(top.map { logits[$0.0] }, top.map { $0.1 }, top.map { Double($0.0) + $0.1 })
let pairs: [(String, Double, Int)] = [("a", 0.5, 1), ("b", 1.25, 2)]
print(pairs.map { "\($0.0)\($0.2):\($0.1 * Double($0.2))" })
print(top.filter { $0.1 > 1 }.count, top.reduce(0.0) { $0 + $1.1 })
