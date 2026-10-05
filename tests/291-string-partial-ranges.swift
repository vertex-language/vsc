// A String sliced by partial ranges of String.Index, and Int(_:) of the
// Substring that makes.
let s = "key=42"
let i = s.firstIndex(of: "=")!
let k = s[..<i]
let v = s[s.index(after: i)...]
print(k, v, s[...i])
if let n = Int(v) { print(n + 1) }
print(Int(s[..<i]) == nil)
