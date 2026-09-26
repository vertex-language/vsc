// An argument made before a later argument throws is released on the way
// out, whichever kind of call it was going to: a memberwise initializer, a
// function, a method.
struct E: Error {}
final class W { let n: Int; init(_ n: Int) { self.n = n }; deinit { print("free", n) } }
struct R { var w: W; var k: Int }

func fail(_ n: Int) throws -> Int {
    if n < 0 { throw E() }
    return n
}
func take(_ a: W, _ b: Int) -> Int { return a.n + b }
struct S {
    func take(_ a: W, _ b: Int, _ c: W) -> Int { return a.n + b + c.n }
}

do { let r = R(w: W(1), k: try fail(5)); print("made", r.k) } catch { print("never") }
do { _ = R(w: W(2), k: try fail(-1)); print("never") } catch { print("caught 1") }
do { _ = take(W(3), try fail(-1)); print("never") } catch { print("caught 2") }
do { _ = S().take(W(4), try fail(-1), W(5)); print("never") } catch { print("caught 3") }
print("done")
