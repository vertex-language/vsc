// Tuples compare element by element, left to right.
print((1, "b") < (2, "a"), (1, "b") < (1, "c"), (3, 3) == (3, 3), (1, 2, 3) > (1, 2, 2))
let pairs = [(2, "b"), (1, "z"), (2, "a")]
for p in pairs.sorted(by: <) { print(p.0, p.1) }
