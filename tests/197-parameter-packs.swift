// Variadic generics: a parameter pack of any length and any types.
func describeAll<each T>(_ value: repeat each T) -> [String] {
    var out: [String] = []
    for v in repeat each value { out.append("\(v)") }
    return out
}
func pairUp<each A, each B>(_ a: repeat each A, with b: repeat each B) -> (repeat (each A, each B)) {
    (repeat (each a, each b))
}
print(describeAll(1, "two", 3.0, true))
print(describeAll())
let pairs = pairUp(1, "x", with: true, 2.5)
print(pairs.0, pairs.1)
