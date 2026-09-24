// String interpolation of values, expressions and nested strings.
let name = "vsc"
let n = 3
let d = 2.5
print("\(name) has \(n) parts at \(d) each: \(Double(n) * d)")
print("nested: \("inner \(n + 1)")")
print("bool \(n > 2), optional \(Int("x") ?? -1), array \([1, 2])")
