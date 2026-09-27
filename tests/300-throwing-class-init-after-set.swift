// A class initializer that throws after its properties are all set: the
// instance is complete, so its deinit runs and its properties are released
// (rung 269 throws before that point).
struct E: Error {}
final class T { let n: Int; init(_ n: Int) { self.n = n }; deinit { print("free", n) } }
final class C {
    let a: T
    let b: T
    init(fail: Bool) throws {
        a = T(1)
        b = T(2)
        if fail { throw E() }
    }
    deinit { print("deinit C") }
}
do { _ = try C(fail: true) } catch { print("caught") }
do { _ = try C(fail: false); print("made") } catch { print("never") }
