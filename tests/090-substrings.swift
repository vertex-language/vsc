// Substrings: prefix, suffix, dropping, and indices.
let s = "abcdefgh"
print(s.prefix(3), s.suffix(2), s.dropFirst(2), s.dropLast(3))
let i = s.index(s.startIndex, offsetBy: 2)
let j = s.index(i, offsetBy: 3)
let sub = s[i..<j]
print(sub, String(sub).count, s[s.index(before: s.endIndex)])
print("a,b,,c".split(separator: ",", omittingEmptySubsequences: false))
