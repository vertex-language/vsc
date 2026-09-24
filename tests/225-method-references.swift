// A method named without calling it: bound to an instance, or unapplied on the type.
struct Counter {
    var n: Int
    func plus(_ k: Int) -> Int { n + k }
}
let c = Counter(n: 10)
let bound = c.plus
let unbound = Counter.plus
print(bound(5), unbound(Counter(n: 1))(1))
print(["a", "b"].map { $0.uppercased() }, [3, 1, 2].sorted(by: <), ["x", "yy"].map(\.count))
let lower = String.lowercased
print(lower("ABC")())
