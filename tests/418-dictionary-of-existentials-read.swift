// A dictionary whose values are existentials, read with if let and
// subscripts.
protocol Service { func name() -> String }
struct Clock: Service { func name() -> String { "clock" } }
final class Pipe: Service {
    func name() -> String { "pipe" }
    deinit { print("pipe freed") }
}
protocol Ref: AnyObject { var id: Int { get } }
final class R: Ref { let id: Int; init(_ id: Int) { self.id = id } }
do {
    var m: [String: any Service] = [:]
    m["a"] = Clock()
    m["b"] = Pipe()
    if let s = m["a"] { print(s.name()) }
    if let s = m["b"] { print(s.name()) }
    print(m["c"] == nil)
    let refs: [String: any Ref] = ["x": R(7)]
    if let r = refs["x"] { print(r.id) }
}
print("done")
