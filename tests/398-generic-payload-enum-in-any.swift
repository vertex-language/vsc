// A generic enum with payloads, a T and an error, stored as Any, copied,
// and cast back; a class payload is freed once.
enum Load<T> {
    case loading
    case ready(T)
    case failed(Error)
}
struct Oops: Error {}
final class Thing {
    let n: Int
    init(_ n: Int) { self.n = n }
    deinit { print("thing", n, "freed") }
}
func describe(_ x: Any) -> String {
    if let l = x as? Load<Int> {
        switch l {
        case .loading: return "loading"
        case .ready(let v): return "ready \(v)"
        case .failed(let e): return "failed \(e)"
        }
    }
    if let l = x as? Load<Thing>, case .ready(let t) = l { return "thing \(t.n)" }
    return "other"
}
var xs: [Any] = [Load<Int>.ready(3), Load<Int>.loading, Load<Int>.failed(Oops()), Load<String>.ready("s")]
let copy = xs
for x in copy { print(describe(x)) }
xs.append(Load<Thing>.ready(Thing(9)))
print(describe(xs[4]))
xs.removeAll()
print("done")
