// A class's async initializer.
func value(_ n: Int) async -> Int { n * 2 }
final class C {
    let n: Int
    init(_ m: Int) async { n = await value(m) }
}
let c = await C(21)
print(c.n)
