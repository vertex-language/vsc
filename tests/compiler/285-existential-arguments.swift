// A value passed to an existential parameter goes into a temporary the
// call borrows, which the caller destroys after it: a struct holding a
// reference and a String, a class, a copy of a local, a cast, two at once,
// and to a method. Over and over, so a temporary destroyed twice crashes.

protocol P { func f() -> Int }
final class K { var v = 1 }
struct S: P { var k = K(); var s = "hello world, longer than inline"; func f() -> Int { return k.v + s.count } }
final class C: P { var v = 2; func f() -> Int { return v } }
func use(_ p: any P) -> Int { return p.f() }
func two(_ p: any P, _ q: P) -> Int { return p.f() + q.f() }
struct Holder { func take(_ p: any P) -> Int { return p.f() } }
func main() -> Int32 {
    var t = 0
    let local: any P = S()
    for _ in 0..<1000 {
        t += use(S())
        t += use(C())
        t += use(local)
        t += use(S() as any P)
        t += two(S(), C())
        t += Holder().take(S())
        let inner: any P = C()
        t += use(inner)
    }
    return Int32(t % 256)
}
