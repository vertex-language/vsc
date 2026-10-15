// An existential declared without a value and given one on each branch,
// in an async function: the first assignment has nothing to let go of,
// whatever an earlier frame left where the variable lives, and the scope
// lets go of the last value.
protocol P: AnyObject { func f() -> Int }
final class A: P {
    let n: Int
    init(_ n: Int) { self.n = n }
    deinit { print("freed A", n) }
    func f() -> Int { return n }
}
final class B: P {
    let n: Int
    init(_ n: Int) { self.n = n }
    deinit { print("freed B", n) }
    func f() -> Int { return n * 10 }
}
func tick(_ n: Int) async -> Int { return n }
// dirty leaves two existentials' words in the memory pick's frame reuses.
func dirty(_ k: Int) async -> Int {
    let y: P = A(k)
    let z: P = B(k)
    return y.f() + z.f() + (await tick(0))
}
func pick(_ k: Int) async throws -> Int {
    var x: P
    if await tick(k) > 2 {
        x = A(k)
    } else {
        x = B(k)
    }
    let t = await tick(1)
    print("using", x.f())
    return x.f() + t
}
print(await dirty(7))
print(try await pick(1))
print(await dirty(8))
print(try await pick(5))
