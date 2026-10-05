// Substrings sliced as Strings are: dropFirst, dropLast, prefix, suffix,
// index arithmetic and range subscripts in the base's indices; and a
// String compared with a Substring.
let s = "enc 1 474 287\ndec 2069\nhéllo wörld"
let lines = s.split(separator: "\n")
print(lines[0].dropFirst(4), lines[1].dropLast(2), lines[2].prefix(3), lines[2].suffix(5))
print("dec 2069" == lines[1], lines[1] == "dec 2069", "x" != lines[0], lines[2].dropFirst(2).dropLast(1))
let w = lines[2]
let i = w.index(w.startIndex, offsetBy: 6)
print(w[i..<w.endIndex], w[w.startIndex...w.index(after: w.startIndex)], w.distance(from: w.startIndex, to: i), w[i])
