// An array as an existential of a protocol it conforms to by extension.
protocol Counted { var n: Int { get } }
extension Array: Counted { var n: Int { count } }
let p: any Counted = [1, 2]
print(p.n)
let ps: [any Counted] = [[1], ["a", "b", "c"]]
print(ps.map { $0.n })
