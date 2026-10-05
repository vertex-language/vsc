// A protocol extension's method called on an existential.
protocol Tok { func piece(_ i: Int) -> String }
extension Tok {
    func decode(_ ids: [Int]) -> String { ids.map { piece($0) }.joined() }
}
struct A: Tok { func piece(_ i: Int) -> String { String(i) } }
struct B: Tok { func piece(_ i: Int) -> String { "<\(i)>" } }
let ts: [any Tok] = [A(), B()]
for t in ts { print(t.decode([1, 2, 3])) }
let t: any Tok = B()
print(t.decode([4]))
