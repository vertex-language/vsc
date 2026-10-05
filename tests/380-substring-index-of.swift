// A Substring finds a Character as a String does, its indices the
// base's: what is cut at them is the same text either way.
let line = "key=value=more; rest"
let tail = line.dropFirst(4)
if let eq = tail.firstIndex(of: "=") {
    print(tail[..<eq], tail[tail.index(after: eq)...])
    print(line[eq...])
}
if let last = tail.lastIndex(of: "e") {
    print(tail[last...])
}
print(tail.firstIndex(of: "k") == nil, tail.contains(";"), tail.contains("k"))
let words = "a:b c:d".split(separator: " ")
for w in words {
    if let c = w.firstIndex(of: ":") { print(w[..<c], w[w.index(after: c)...]) }
}
