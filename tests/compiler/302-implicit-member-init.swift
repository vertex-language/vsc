// `.init(...)` where a value of some type is wanted -- an annotated let,
// an argument, an optional of it -- is that type's initializer, whichever
// one the arguments pick, memberwise or declared, of a struct or a class.

struct S { var n = 3; init() {} ; init(n: Int) { self.n = n } }
struct M { var a: Int; var b: String }
final class C { var v: Int; init(v: Int) { self.v = v } }
func take(_ s: S) -> Int { return s.n }
func main() -> Int32 {
    let s: S = .init()
    let t: S = .init(n: 8)
    let m: M = .init(a: 1, b: "x")
    let c: C = .init(v: 5)
    let o: S? = .init(n: 2)
    var arr: [S] = []
    arr.append(.init(n: 4))
    print(s.n, t.n, m.a, m.b, c.v, o!.n, take(.init(n: 6)), arr[0].n)
    return Int32(t.n)
}
