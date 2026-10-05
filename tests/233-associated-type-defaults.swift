// An associated type with a default, and one inferred from a witness.
protocol Store {
    associatedtype Key: Hashable = String
    associatedtype Value
    func get(_ k: Key) -> Value?
}
struct Names: Store {
    let data = ["a": "Ann"]
    func get(_ k: String) -> String? { data[k] }
}
struct Squares: Store {
    typealias Key = Int
    func get(_ k: Int) -> Int? { k >= 0 ? k * k : nil }
}
func fetch<S: Store>(_ s: S, _ k: S.Key) -> String { s.get(k).map { "\($0)" } ?? "none" }
print(fetch(Names(), "a"), fetch(Names(), "b"), fetch(Squares(), 7), fetch(Squares(), -1))
