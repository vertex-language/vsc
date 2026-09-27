// A generic function calls a requirement its conformer overloads by type:
// each call takes the overload its argument picks.
protocol Sink {
    mutating func put(_ n: Int)
    mutating func put(_ s: String)
}
struct T: Sink {
    var log: [String] = []
    mutating func put(_ n: Int) { log.append("int \(n)") }
    mutating func put(_ s: String) { log.append("str \(s)") }
}
func fill<S: Sink>(_ s: inout S) { s.put(1); s.put("a") }
var t = T()
fill(&t)
print(t.log)
