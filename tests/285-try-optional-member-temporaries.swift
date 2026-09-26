// `try? f().member`: f's result is a temporary of the expression, made
// only where f returned. It is released there, before the success and
// failure paths meet -- never on the failure path, which has no result.
struct E: Error {}
final class W {
    let n: Int
    init(_ n: Int) { self.n = n }
    deinit { print("free", n) }
}
func make(_ ok: Bool, _ base: Int) throws -> [W] {
    if !ok { throw E() }
    return [W(base), W(base + 1)]
}
let a = try? make(true, 1).first?.n
print("a", a ?? -1)
let b = try? make(false, 10).count
print("b", b ?? -1)
func inside() {
    let c = try? make(true, 20).last?.n
    print("c", c ?? -1)
    let d = try? make(false, 30).first
    print("d", d == nil)
}
inside()
print("done")
