// Requirements overloaded by type, further: called in a generic function
// and through an existential, where each overload has a witness row of
// its own.
protocol Sink {
    mutating func put(_ n: Int)
    mutating func put(_ s: String)
    func show(_ d: Double) -> String
    func show(_ b: Bool) -> String
}
struct T: Sink {
    var log: [String] = []
    mutating func put(_ n: Int) { log.append("int \(n)") }
    mutating func put(_ s: String) { log.append("str \(s)") }
    func show(_ d: Double) -> String { "d\(d)" }
    func show(_ b: Bool) -> String { "b\(b)" }
}
func fill<S: Sink>(_ s: inout S) { s.put(1); s.put("a"); print(s.show(true), s.show(1.5)) }
var t = T()
fill(&t)
var e: any Sink = T()
e.put("x"); e.put(2)
print(t.log, e.show(false), e.show(2.5))
