// A tuple element made before a later element throws is released on the
// way out, as a call's argument is (rung 282).
struct E: Error {}
final class W {
    let n: Int
    init(_ n: Int) { self.n = n }
    deinit { print("free", n) }
}
func fail(_ n: Int) throws -> W {
    if n < 0 { throw E() }
    return W(n)
}
func pair(_ a: Int, _ b: Int) throws -> (W, W) {
    return (try fail(a), try fail(b))
}
do { let p = try pair(1, 2); print("made", p.0.n, p.1.n) } catch { print("never") }
do { _ = try pair(3, -1); print("never") } catch { print("caught") }
func takes(_ t: (W, W)) -> Int { return t.0.n + t.1.n }
do { _ = takes((W(4), try fail(-1))); print("never") } catch { print("caught 2") }
print("done")
