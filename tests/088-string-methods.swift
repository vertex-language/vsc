// Everyday String methods.
let s = "Hello, World"
print(s.hasPrefix("Hell"), s.hasSuffix("ld"), s.contains("lo, W"), s.contains("x"))
print(s.uppercased(), s.lowercased())
print(s.replacing("l", with: "L"))
print(s.split(separator: ",").map { String($0) })
print(s.firstIndex(of: "W").map { s.distance(from: s.startIndex, to: $0) } ?? -1)
