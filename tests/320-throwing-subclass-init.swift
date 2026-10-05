// Throwing class initializers, further: a subclass's, throwing before its
// super.init (unfinished: what it set is let go of, no deinit) and after
// it (complete: deinit, the subclass's then the superclass's), and a
// class whose optional property starts as nil.
struct E: Error {}
final class T { let n: Int; init(_ n: Int) { self.n = n }; deinit { print("free", n) } }
func check(_ ok: Bool) throws { if !ok { throw E() } }
class Base { let t: T; init() { t = T(0) }; deinit { print("deinit Base") } }
class Sub: Base {
    let u: T
    init(stage: Int) throws {
        u = T(stage)
        if stage == 1 { throw E() }
        super.init()
        try check(stage != 2)
    }
    deinit { print("deinit Sub") }
}
for s in 1...3 {
    do { _ = try Sub(stage: s); print("made", s) } catch { print("caught", s) }
}
final class Opt { var a: T?; let b: T
    init(_ f: Bool) throws { b = T(9); if f { throw E() } }
    deinit { print("deinit Opt") } }
do { _ = try Opt(true) } catch { print("caught opt") }
