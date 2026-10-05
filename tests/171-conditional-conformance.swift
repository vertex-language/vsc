// A generic type conforms to a protocol only when its parameter does.
protocol Summary { func summary() -> String }
extension Int: Summary { func summary() -> String { "int \(self)" } }
struct Pair<T> { let a: T, b: T }
extension Pair: Summary where T: Summary {
    func summary() -> String { "(\(a.summary()), \(b.summary()))" }
}
extension Pair: Equatable where T: Equatable {}
extension Array: Summary where Element: Summary {
    func summary() -> String { map { $0.summary() }.joined(separator: "; ") }
}
print(Pair(a: 1, b: 2).summary(), [3, 4].summary())
print(Pair(a: 1, b: 2) == Pair(a: 1, b: 2), Pair(a: "x", b: "y") == Pair(a: "x", b: "z"))
