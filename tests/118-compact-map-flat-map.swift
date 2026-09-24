// compactMap drops nils; flatMap concatenates.
let raw = ["1", "x", "3", "", "5"]
print(raw.compactMap { Int($0) })
print([[1, 2], [], [3]].flatMap { $0 })
print([1, 2, 3].flatMap { Array(repeating: $0, count: $0) })
print([Int?.none, 4, nil, 6].compactMap { $0 })
