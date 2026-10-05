// Constraints on generic parameters, in angle brackets and where clauses.
func indexOf<T: Equatable>(_ x: T, in xs: [T]) -> Int? {
    for (i, v) in xs.enumerated() where v == x { return i }
    return nil
}
func allEqual<A: Sequence, B: Sequence>(_ a: A, _ b: B) -> Bool
    where A.Element == B.Element, A.Element: Equatable {
    Array(a) == Array(b)
}
extension Array where Element: Numeric {
    func total() -> Element { reduce(0, +) }
}
print(indexOf("c", in: ["a", "b", "c"]) as Any, indexOf(9, in: [1]) as Any)
print(allEqual([1, 2, 3], 1...3), allEqual("ab", ["a", "c"]))
print([1, 2, 3].total(), [0.5, 0.25].total())
