// Joining strings and building them from sequences.
let words = ["one", "two", "three"]
print(words.joined(), words.joined(separator: ", "), words.joined(separator: "\n"))
print([1, 2, 3].map(String.init).joined(separator: "+"))
print(String(["a", "b", "c"].reversed().joined()), [["x"], ["y", "z"]].joined().count)
