// A try? passed straight to a call is a temporary the call borrows: it
// ends with its statement, as a plain try's result does.
enum E: Error { case no }
final class C {
    let s: String
    init(_ s: String) { self.s = s }
    deinit { print("freed", s) }
}
struct Box { var n: Int; init(_ c: C?) { n = c?.s.count ?? -1 } }
func make(_ s: String, _ ok: Bool = true) throws -> C {
    if !ok { throw E.no }
    return C(s)
}
func use() throws -> Int {
    let b = Box(try make("plain"))
    print("after plain")
    let c = Box(try? make("optional"))
    print("after optional")
    let d = Box(try? make("failed", false))
    print("after failed")
    let kept = try? make("kept")
    print("kept", kept?.s ?? "")
    return b.n + c.n + d.n
}
print(try use())
