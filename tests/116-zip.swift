// zip pairs two sequences and stops at the shorter one.
let names = ["a", "b", "c", "d"]
let scores = [90, 85, 70]
for (n, s) in zip(names, scores) { print(n, s) }
print(zip(1..., names).map { "\($0)\($1)" })
print(Dictionary(uniqueKeysWithValues: zip(names, scores)).count)
