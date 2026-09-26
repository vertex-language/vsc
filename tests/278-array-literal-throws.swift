// An array literal whose element throws ends the elements made before it.
struct E: Error {}
final class W { let n: Int; init(_ n: Int) { self.n = n }; deinit { print("free", n) } }
func make(_ n: Int) throws -> W {
    if n == 3 { throw E() }
    return W(n)
}
func sum(_ ws: [W]) -> Int { return ws.reduce(0) { $0 + $1.n } }
func f(_ k: Int) throws -> Int {
    return sum([try make(1), try make(k), try make(2)])
}
print(try f(5))
do { _ = try f(3) } catch { print("thrown") }
