// A primary associated type constrains an opaque or existential type in angle brackets.
protocol Source<Value> {
    associatedtype Value
    mutating func next() -> Value
}
struct Counter: Source {
    var n = 0
    mutating func next() -> Int { n += 1; return n }
}
struct Echo: Source {
    let word: String
    mutating func next() -> String { word }
}
func take<S: Source>(_ s: inout S, _ k: Int) -> [S.Value] { (0..<k).map { _ in s.next() } }
var c: some Source<Int> = Counter()
print(take(&c, 3))
var sources: [any Source<String>] = [Echo(word: "a"), Echo(word: "b")]
print(sources[1].next())
func sum(_ xs: some Collection<Int>) -> Int { xs.reduce(0, +) }
print(sum([1, 2, 3]), sum(1...10), sum(Set([5])))
